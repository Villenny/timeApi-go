package timeApi

import (
	"context"
	"reflect"
	"sync"
	"time"
)

// convenience wrapper for context.WithCancel
func withCancel(parent context.Context) (ctx context.Context, cancel context.CancelFunc) {
	return context.WithCancel(parent)
}

// convenience wrapper for withDeadline
func withTimeout(parent context.Context, timeapi TimeApi, timeout time.Duration) (context.Context, context.CancelFunc) {
	return withDeadline(parent, timeapi, timeapi.Now().Add(timeout))
}

// ////////////////////////////////////////////////////////////////////////////////////////////

// withDeadline returns a copy of the parent context with the deadline adjusted
// to be no later than d. If the parent's deadline is already earlier than d,
// withDeadline(parent, d) is semantically equivalent to parent. The returned
// context's Done channel is closed when the deadline expires, when the returned
// cancel function is called, or when the parent context's Done channel is
// closed, whichever happens first.
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete.
func withDeadline(parent context.Context, timeapi TimeApi, d time.Time) (context.Context, context.CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if cur, ok := parent.Deadline(); ok && cur.Before(d) {
		// The current deadline is already sooner than the new one.
		return withCancel(parent)
	}

	ctx, cancelFn := withCancel(parent)

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

// A timerCtx carries a timer and a deadline. It wraps a system cancel context (cancelCtx) around its parent
// to implement the cancel and notification of children and such. It then implements Done and Err.
// It implements cancel by stopping its timer then delegating to cancelCtx.cancel.
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

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

type stringer interface {
	String() string
}

// helper function, get the name of a context via the public String() method thats not in the interface
func ContextName(c context.Context) string {
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflect.TypeOf(c).String()
}

// its odd this isnt in the interface, gets a debug name for a context
func (c *timerCtx) String() string {
	return ContextName(c.parent) + ".WithDeadline(" +
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

// Done returns a channel that's closed when work done on behalf of this
// context should be canceled. Done may return nil if this context can
// never be canceled. Successive calls to Done return the same value.
// The close of the Done channel may happen asynchronously,
// after the cancel function returns.
//
// WithCancel arranges for Done to be closed when cancel is called;
// WithDeadline arranges for Done to be closed when the deadline
// expires; WithTimeout arranges for Done to be closed when the timeout
// elapses.
//
// Done is provided for use in select statements:
//
//	// Stream generates values with DoSomething and sends them to out
//	// until DoSomething returns an error or ctx.Done is closed.
//	func Stream(ctx context.Context, out chan<- Value) error {
//		for {
//			v, err := DoSomething(ctx)
//			if err != nil {
//				return err
//			}
//			select {
//			case <-ctx.Done():
//				return ctx.Err()
//			case out <- v:
//			}
//		}
//	}
//
// See https://blog.golang.org/pipelines for more examples of how to use
// a Done channel for cancellation.
func (c *timerCtx) Done() <-chan struct{} { return c.done }

// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
// After Err returns a non-nil error, successive calls to Err return the same error.
func (c *timerCtx) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
//
// Use context values only for request-scoped data that transits
// processes and API boundaries, not for passing optional parameters to
// functions.
//
// A key identifies a specific value in a Context. Functions that wish
// to store values in Context typically allocate a key in a global
// variable then use that key as the argument to context.WithValue and
// Context.Value. A key can be any type that supports equality;
// packages should define keys as an unexported type to avoid
// collisions.
//
// Packages that define a Context key should provide type-safe accessors
// for the values stored using that key:
//
//	// Package user defines a User type that's stored in Contexts.
//	package user
//
//	import "context"
//
//	// User is the type of value stored in the Contexts.
//	type User struct {...}
//
//	// key is an unexported type for keys defined in this package.
//	// This prevents collisions with keys defined in other packages.
//	type key int
//
//	// userKey is the key for user.User values in Contexts. It is
//	// unexported; clients use user.NewContext and user.FromContext
//	// instead of using this key directly.
//	var userKey key
//
//	// NewContext returns a new Context that carries value u.
//	func NewContext(ctx context.Context, u *User) context.Context {
//		return context.WithValue(ctx, userKey, u)
//	}
//
//	// FromContext returns the User value stored in ctx, if any.
//	func FromContext(ctx context.Context) (*User, bool) {
//		u, ok := ctx.Value(userKey).(*User)
//		return u, ok
//	}
func (c *timerCtx) Value(key interface{}) interface{} { return c.parent.Value(key) }
