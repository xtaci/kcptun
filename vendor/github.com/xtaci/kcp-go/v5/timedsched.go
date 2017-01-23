// The MIT License (MIT)
//
// Copyright (c) 2015 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package kcp

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// SystemTimedSched is the library-level timed scheduler, shared by all sessions.
// It drives periodic KCP flush()/update() calls, avoiding one goroutine per session.
var SystemTimedSched *TimedSched = NewTimedSched(min(runtime.NumCPU(), 2))

type timedFunc struct {
	execute func()
	ts      time.Time
}

// timedFuncHeap is a min-heap by deadline. It is typed rather than built on
// container/heap, whose any-typed Push and Pop can allocate for each task.
type timedFuncHeap []timedFunc

func (h timedFuncHeap) Len() int { return len(h) }

func (h *timedFuncHeap) push(f timedFunc) {
	*h = append(*h, f)
	s := *h
	for i := len(s) - 1; i > 0; {
		p := (i - 1) / 2
		if !s[i].ts.Before(s[p].ts) {
			break
		}
		s[i], s[p] = s[p], s[i]
		i = p
	}
}

func (h *timedFuncHeap) pop() timedFunc {
	s := *h
	n := len(s) - 1
	top := s[0]
	s[0] = s[n]
	s[n] = timedFunc{} // clear to avoid memory leak (both execute and ts)
	s = s[:n]
	for i := 0; ; {
		l, m := 2*i+1, i
		if l < n && s[l].ts.Before(s[m].ts) {
			m = l
		}
		if r := l + 1; r < n && s[r].ts.Before(s[m].ts) {
			m = r
		}
		if m == i {
			break
		}
		s[i], s[m] = s[m], s[i]
		i = m
	}
	*h = s
	return top
}

// schedSlack is how early a task may run so that one wakeup serves several
// tasks. It is half of KCP's shortest update interval, 10ms.
const schedSlack = 5 * time.Millisecond

// testHookBeforeArm runs in a shard just before it arms its timer.
var testHookBeforeArm atomic.Pointer[func()]

// TimedSched runs functions at given times on a few shard goroutines.
//
// Put pushes onto a shard's heap under its lock and wakes the shard only when
// the new task is due before everything already queued there. A shard runs
// all tasks due within schedSlack per wakeup, outside its lock, so a task may
// Put itself again. Previously, each Put passed through a feeder goroutine
// and an unbuffered channel and reset a timer, causing several scheduling
// handoffs per task.
type TimedSched struct {
	shards []tsShard
	next   atomic.Uint32

	dieOnce sync.Once
	die     chan struct{}
}

type tsShard struct {
	mu    sync.Mutex
	tasks timedFuncHeap
	// armed is the deadline the shard goroutine sleeps until, zero if none.
	armed time.Time
	wake  chan struct{}
}

// NewTimedSched starts a scheduler with parallel shards.
func NewTimedSched(parallel int) *TimedSched {
	ts := &TimedSched{
		shards: make([]tsShard, max(parallel, 1)),
		die:    make(chan struct{}),
	}
	for i := range ts.shards {
		ts.shards[i].wake = make(chan struct{}, 1)
		go ts.run(&ts.shards[i])
	}
	return ts
}

func (ts *TimedSched) run(sh *tsShard) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	var due []timedFunc
	for {
		sh.mu.Lock()
		limit := time.Now().Add(schedSlack)
		for len(sh.tasks) > 0 && !sh.tasks[0].ts.After(limit) {
			due = append(due, sh.tasks.pop())
		}
		var wait time.Duration
		pending := false
		sh.armed = time.Time{}
		if len(due) == 0 && len(sh.tasks) > 0 {
			if hook := testHookBeforeArm.Load(); hook != nil {
				(*hook)()
			}
			sh.armed = sh.tasks[0].ts
			wait = time.Until(sh.armed)
			pending = true
		}
		sh.mu.Unlock()

		if len(due) > 0 {
			for k := range due {
				due[k].execute()
				due[k] = timedFunc{}
			}
			due = due[:0]
			continue
		}

		// A head that came due while arming, as after a GC pause, fires at once.
		// Without a timer no Put could wake the shard for it.
		var fire <-chan time.Time
		if pending {
			timer.Reset(max(wait, 0))
			fire = timer.C
		}
		select {
		case <-fire:
		case <-sh.wake:
			if fire != nil && !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-ts.die:
			timer.Stop()
			return
		}
	}
}

// Put schedules f to run at deadline.
func (ts *TimedSched) Put(f func(), deadline time.Time) {
	sh := &ts.shards[ts.next.Add(1)%uint32(len(ts.shards))]
	sh.mu.Lock()
	sh.tasks.push(timedFunc{f, deadline})
	// The shard is busy or sleeping until something sooner; only an
	// earlier head needs it awake.
	wake := sh.tasks[0].ts.Equal(deadline) && (sh.armed.IsZero() || deadline.Before(sh.armed))
	sh.mu.Unlock()
	if wake {
		select {
		case sh.wake <- struct{}{}:
		default:
		}
	}
}

// Close stops the scheduler. Pending tasks are dropped.
func (ts *TimedSched) Close() { ts.dieOnce.Do(func() { close(ts.die) }) }
