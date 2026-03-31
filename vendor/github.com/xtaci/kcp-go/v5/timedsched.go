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
	"container/heap"
	"runtime"
	"sync"
	"time"
)

// SystemTimedSched is the library-level timed scheduler, shared by all sessions.
// It drives periodic KCP flush()/update() calls, avoiding one goroutine per session.
var SystemTimedSched *TimedSched = NewTimedSched(max(runtime.NumCPU(), 2))

type timedFunc struct {
	execute func()
	ts      time.Time
}

// a heap for sorted timed function
type timedFuncHeap []timedFunc

func (h timedFuncHeap) Len() int           { return len(h) }
func (h timedFuncHeap) Less(i, j int) bool { return h[i].ts.Before(h[j].ts) }
func (h timedFuncHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *timedFuncHeap) Push(x any)        { *h = append(*h, x.(timedFunc)) }
func (h *timedFuncHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = timedFunc{} // clear to avoid memory leak (both execute and ts)
	*h = old[:n-1]
	return x
}

// TimedSched is a two-stage parallel scheduler for timed task execution.
//
// Architecture (two-stage pipeline):
//
//	Stage 1 - "prepend" goroutine:
//	  External callers submit tasks via Put(). Tasks are appended to a shared
//	  slice under a mutex (fast, non-blocking). The prepend goroutine drains
//	  this slice and feeds tasks one-by-one into chTask.
//
//	Stage 2 - "sched" goroutines (N = NumCPU):
//	  Each sched goroutine maintains a local min-heap of pending tasks.
//	  It receives tasks from chTask, executes overdue ones immediately,
//	  and uses a timer for the earliest future task.
//
// Why two stages?
//   - Stage 1 decouples callers from the scheduler's internal heap,
//     ensuring Put() never blocks on heap operations.
//   - Stage 2 runs in parallel, distributing timer-driven work across CPUs.
type TimedSched struct {
	// Stage 1: task collection
	prependTasks    []timedFunc
	prependLock     sync.Mutex
	chPrependNotify chan struct{}

	// Stage 2: parallel execution
	chTask chan timedFunc

	dieOnce sync.Once
	die     chan struct{}
}

// NewTimedSched creates a parallel-scheduler with given parallelization
func NewTimedSched(parallel int) *TimedSched {
	ts := new(TimedSched)
	ts.chTask = make(chan timedFunc)
	ts.die = make(chan struct{})
	ts.chPrependNotify = make(chan struct{}, 1)

	for range parallel {
		go ts.sched()
	}
	go ts.prepend()
	return ts
}

// sched is a worker goroutine (Stage 2) that manages a local min-heap
// of timed tasks. It executes tasks when their deadline arrives.
func (ts *TimedSched) sched() {
	timer := time.NewTimer(0)
	defer timer.Stop()

	var tasks timedFuncHeap
	drained := false
	for {
		select {
		case task := <-ts.chTask:
			now := time.Now()
			if now.After(task.ts) {
				// already delayed! execute immediately
				task.execute()
			} else {
				heap.Push(&tasks, task)
				// properly reset timer to trigger based on the top element
				stopped := timer.Stop()
				if !stopped && !drained {
					<-timer.C
				}
				timer.Reset(tasks[0].ts.Sub(now))
				drained = false
			}
		case now := <-timer.C:
			drained = true
			for tasks.Len() > 0 {
				if now.After(tasks[0].ts) {
					heap.Pop(&tasks).(timedFunc).execute()
				} else {
					timer.Reset(tasks[0].ts.Sub(now))
					drained = false
					break
				}
			}
		case <-ts.die:
			return
		}
	}
}

// prepend is the Stage 1 goroutine that collects externally submitted tasks
// and feeds them into the Stage 2 worker pool via chTask.
func (ts *TimedSched) prepend() {
	var tasks []timedFunc
	for {
		select {
		case <-ts.chPrependNotify:
			ts.prependLock.Lock()
			// swap slices to minimize time under lock
			tasks, ts.prependTasks = ts.prependTasks, tasks[:0]
			ts.prependLock.Unlock()

			for k := range tasks {
				select {
				case ts.chTask <- tasks[k]:
					tasks[k] = timedFunc{} // clear to avoid memory leak
				case <-ts.die:
					return
				}
			}
			tasks = tasks[:0]
		case <-ts.die:
			return
		}
	}
}

// Put a function 'f' awaiting to be executed at 'deadline'
func (ts *TimedSched) Put(f func(), deadline time.Time) {
	ts.prependLock.Lock()
	ts.prependTasks = append(ts.prependTasks, timedFunc{f, deadline})
	ts.prependLock.Unlock()

	select {
	case ts.chPrependNotify <- struct{}{}:
	default:
	}
}

// Close terminates this scheduler
func (ts *TimedSched) Close() { ts.dieOnce.Do(func() { close(ts.die) }) }
