// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package deadline provides deadline timer used to implement
// net.Conn compatible connection
package deadline

import (
	"context"
	"sync"
	"time"
)

type deadlineState uint8

const (
	deadlineSuspended deadlineState = iota
	deadlineStarted
	deadlineExceeded
)

//nolint:gochecknoglobals // A canceled context is immutable, so it is safe to share.
var exceededContext context.Context

func init() {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(context.DeadlineExceeded)
	exceededContext = ctx
}

// Deadline signals updatable deadline timer.
type Deadline struct {
	mu     sync.RWMutex
	timer  timer
	done   chan struct{}
	ctx    context.Context //nolint:containedctx // Deadline state, not a request scope.
	cancel context.CancelCauseFunc

	deadline time.Time
	state    deadlineState
	pending  uint8
}

// New creates new deadline timer.
func New() *Deadline {
	return &Deadline{
		done: make(chan struct{}),
	}
}

func (d *Deadline) timeout() {
	d.mu.Lock()
	if d.pending--; d.pending != 0 || d.state != deadlineStarted {
		d.mu.Unlock()

		return
	}

	cancel := d.fire()
	d.mu.Unlock()

	if cancel != nil {
		cancel(context.DeadlineExceeded)
	}
}

// Returns the cancel func rather than calling it: canceling walks derived
// contexts' closures, which must not run under mu.
func (d *Deadline) fire() context.CancelCauseFunc {
	d.state = deadlineExceeded
	close(d.done)

	cancel := d.cancel
	d.ctx, d.cancel = nil, nil

	return cancel
}

// Context returns a context for the current deadline, canceled with
// context.DeadlineExceeded as its Cause.
func (d *Deadline) Context() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == deadlineExceeded {
		return exceededContext
	}
	if d.ctx == nil {
		d.ctx, d.cancel = context.WithCancelCause(context.Background())
	}

	return d.ctx
}

// Set new deadline. Zero value means no deadline.
func (d *Deadline) Set(setTo time.Time) {
	if cancel := d.set(setTo); cancel != nil {
		cancel(context.DeadlineExceeded)
	}
}

func (d *Deadline) set(setTo time.Time) context.CancelCauseFunc {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == deadlineStarted && d.timer.Stop() {
		d.pending--
	}

	d.deadline = setTo
	d.pending++

	if d.state == deadlineExceeded {
		d.done = make(chan struct{})
	}

	if setTo.IsZero() {
		d.pending--
		d.state = deadlineSuspended

		return nil
	}

	if dur := time.Until(setTo); dur > 0 {
		d.state = deadlineStarted
		if d.timer == nil {
			d.timer = afterFunc(dur, d.timeout)
		} else {
			d.timer.Reset(dur)
		}

		return nil
	}

	d.pending--

	return d.fire()
}

// Done receives deadline signal.
func (d *Deadline) Done() <-chan struct{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.done
}

// Err returns context.DeadlineExceeded if the deadline is exceeded.
// Otherwise, it returns nil.
func (d *Deadline) Err() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.state == deadlineExceeded {
		return context.DeadlineExceeded
	}

	return nil
}

// Deadline returns current deadline.
func (d *Deadline) Deadline() (time.Time, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.deadline.IsZero() {
		return d.deadline, false
	}

	return d.deadline, true
}
