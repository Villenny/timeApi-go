package timeApi

import (
	"context"
	"reflect"
	"sync"
	"time"
)

func WithCancel(parent context.Context) (ctx context.Context, cancel context.CancelFunc) {
	return context.WithCancel(parent)
}

func WithTimeout(parent context.Context, timeapi TimeApi, timeout time.Duration) (context.Context, context.CancelFunc) {
	return WithDeadline(parent, timeapi, timeapi.Now().Add(timeout))
}

func WithValue(parent context.Context, key, val any) context.Context {
	return context.WithValue(parent, key, val)
}

// ////////////////////////////////////////////////////////////////////////////////////////////

// WithDeadline returns a copy of the parent context with the deadline adjusted
// to be no later than d. If the parent's deadline is already earlier than d,
// WithDeadline(parent, d) is semantically equivalent to parent. The returned
// context's Done channel is closed when the deadline expires, when the returned
// cancel function is called, or when the parent context's Done channel is
// closed, whichever happens first.
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete.
func WithDeadline(parent context.Context, timeapi TimeApi, d time.Time) (context.Context, context.CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if cur, ok := parent.Deadline(); ok && cur.Before(d) {
		// The current deadline is already sooner than the new one.
		return WithCancel(parent)
	}

	ctx, cancelFn := WithCancel(parent)

	c := &timerCtx{
		parent:    parent,
		cancelCtx: ctx,
		cancelFn:  cancelFn,
		timeapi:   timeapi,
		deadline:  d,
		mu:        &sync.Mutex{},
		done:      make(chan struct{}),
	}

	dur := timeapi.Until(d)
	if dur <= 0 {
		c.cancel(context.DeadlineExceeded) // deadline has already passed
		return c, func() { c.cancel(context.Canceled) }
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.timer = timeapi.AfterFunc(dur, func() {
			c.cancel(context.DeadlineExceeded)
		})
	}
	return c, func() { c.cancel(context.Canceled) }
}

// /////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// A timerCtx carries a timer and a deadline. It embeds a cancelCtx to
// implement Done and Err. It implements cancel by stopping its timer then
// delegating to cancelCtx.cancel.
type timerCtx struct {
	parent    context.Context
	cancelCtx context.Context
	cancelFn  context.CancelFunc
	deadline  time.Time
	done      chan struct{}
	err       error
	timeapi   TimeApi
	timer     *Timer
	mu        *sync.Mutex
}

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

type stringer interface {
	String() string
}

func contextName(c context.Context) string {
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflect.TypeOf(c).String()
}

func (c *timerCtx) String() string {
	return contextName(c.parent) + ".WithDeadline(" +
		c.deadline.String() + " [" +
		time.Until(c.deadline).String() + "])"
}

func (c *timerCtx) cancel(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		// latch the first error, then return it forever, ie cant cancel after it failed
		return
	}
	c.err = err
	close(c.done)
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.cancelFn()
}

func (c *timerCtx) Done() <-chan struct{} { return c.done }

func (c *timerCtx) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *timerCtx) Value(key interface{}) interface{} { return c.parent.Value(key) }
