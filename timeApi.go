package timeApi

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Re-export of time.Duration
type Duration = time.Duration
type Time = time.Time

/*
UnixNano only works within ±292 years around 1970 (between 1678 and 2262). Also, since maximum duration is ~292 years, even those two will give a clamped result on Sub
*/
var MaxDuration = 1<<63 - 1
var MinTime = time.Unix(-2208988800, 0) // Jan 1, 1900
var MaxTime = MinTime.Add(1<<63 - 1)

// the interface to a ticker, real or fake
type TickerAdapter interface {
	Stop()

	// Reset stops a ticker and resets its period to the specified duration.
	// The next tick will arrive after the new period elapses. The duration d
	// must be greater than zero; if not, Reset will panic.
	Reset(d Duration)
}

// Cant put a channel datamember in an interface, so we need an adapter to wrap and make the channel accessible
type Ticker struct {
	C      <-chan Time   // republished channel from the internal adapter
	ticker TickerAdapter // the real or fake ticker
	name   string        // every ticker is given a unique string id for easier debugging
}

func (t *Ticker) Stop()            { t.ticker.Stop() }
func (t *Ticker) Reset(d Duration) { t.ticker.Reset(d) }

// the interface to a timer, real or fake
type TimerAdapter interface {
	Stop() bool

	// Reset should be invoked only on stopped or expired timers with drained channels.
	//
	// If a program has already received a value from t.C, the timer is known
	// to have expired and the channel drained, so t.Reset can be used directly.
	// If a program has not yet received a value from t.C, however,
	// the timer must be stopped and—if Stop reports that the timer expired
	// before being stopped—the channel explicitly drained:
	//
	//	if !t.Stop() {
	//		<-t.C
	//	}
	//	t.Reset(d)
	Reset(d Duration) bool
}

// Cant put a channel datamember in an interface, so we need an adapter to wrap and make the channel accessible
type Timer struct {
	C     <-chan Time  // republished channel from the internal adapter
	timer TimerAdapter // the real or fake timer
	name  string       // every timer is given a unique string id for easier debugging
}

func (t *Timer) Stop() bool            { return t.timer.Stop() }
func (t *Timer) Reset(d Duration) bool { return t.timer.Reset(d) }

// Inspired by github.com/benbjohnson/clock
// wrap all the global time methods, plus gosched, and the context timeouts
type TimeApi interface {
	After(d time.Duration) <-chan time.Time
	AfterFunc(d time.Duration, f func()) *Timer
	Now() time.Time
	Since(t time.Time) time.Duration
	Until(t time.Time) time.Duration
	Sleep(d time.Duration)
	Gosched()
	Tick(d time.Duration) <-chan time.Time
	NewTicker(d time.Duration) *Ticker
	NewTimer(d time.Duration) *Timer
	WithDeadline(parent context.Context, d time.Time) (context.Context, context.CancelFunc)
	WithTimeout(parent context.Context, t time.Duration) (context.Context, context.CancelFunc)
}

// ///////////////////////////////////////////////////////////////////

// The real time api struct, wraps the system time/context/goshed functions
type RealTimeApi struct {
	nextId int64
}

// New returns an instance of a real-time clock. This is a simple wrapper around the time.* functions in the go standard library
func New() *RealTimeApi {
	return &RealTimeApi{}
}

/* this is REALLY not a great API on the part of go, the lack of stop signals and the resulting leak of resources is unconscionable */

// After waits for the duration to elapse and then sends the current time
// on the returned channel.
// It is equivalent to NewTimer(d).C.
// The underlying Timer is not recovered by the garbage collector
// until the timer fires. If efficiency is a concern, use NewTimer
// instead and call Timer.Stop if the timer is no longer needed.
func (t *RealTimeApi) After(d time.Duration) <-chan time.Time { return time.After(d) }

// AfterFunc waits for the duration to elapse and then calls f
// in its own goroutine. It returns a Timer that can
// be used to cancel the call using its Stop method.
func (t *RealTimeApi) AfterFunc(d time.Duration, f func()) *Timer {
	i := time.AfterFunc(d, f)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Timer{i.C, i, "AfterFunc/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}

// Now returns the current local time.
func (t *RealTimeApi) Now() time.Time { return time.Now() }

// Since returns the time elapsed since t.
// It is shorthand for time.Now().Sub(t).
func (t *RealTimeApi) Since(tm time.Time) time.Duration { return time.Since(tm) }

// Until returns the duration until t.
// It is shorthand for t.Sub(time.Now()).
func (t *RealTimeApi) Until(tm time.Time) time.Duration { return time.Until(tm) }

// Sleep pauses the current goroutine for at least the duration d.
// A negative or zero duration causes Sleep to return immediately.
// Note on windows golang maximum sleep resolution is roughly 2ms, any amount < than that will sleep for that long
func (t *RealTimeApi) Sleep(d time.Duration) { time.Sleep(d) }

// Gosched yields the processor, allowing other goroutines to run. It does not
// suspend the current goroutine, so execution resumes automatically.
// Note gosched typically takes about 45-60ns to run on my machine
func (t *RealTimeApi) Gosched() { runtime.Gosched() }

// Tick is a convenience wrapper for NewTicker providing access to the ticking
// channel only. While Tick is useful for clients that have no need to shut down
// the Ticker, be aware that without a way to shut it down the underlying
// Ticker cannot be recovered by the garbage collector; it "leaks".
// Unlike NewTicker, Tick will return nil if d <= 0.
// Wise programmers should avoid this abomination like the plague
func (t *RealTimeApi) Tick(d time.Duration) <-chan time.Time { return time.Tick(d) }

// NewTicker returns a new Ticker containing a channel that will send
// the current time on the channel after each tick. The period of the
// ticks is specified by the duration argument. The ticker will adjust
// the time interval or drop ticks to make up for slow receivers.
// The duration d must be greater than zero; if not, NewTicker will
// panic. Stop the ticker to release associated resources.
func (t *RealTimeApi) NewTicker(d time.Duration) *Ticker {
	i := time.NewTicker(d)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Ticker{i.C, i, "Ticker/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}

// NewTimer creates a new Timer that will send
// the current time on its channel after at least duration d.
func (t *RealTimeApi) NewTimer(d time.Duration) *Timer {
	i := time.NewTimer(d)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Timer{i.C, i, "Timer/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}

// WithDeadline returns a copy of the parent context with the deadline adjusted
// to be no later than d. If the parent's deadline is already earlier than d,
// WithDeadline(parent, d) is semantically equivalent to parent. The returned
// context's Done channel is closed when the deadline expires, when the returned
// cancel function is called, or when the parent context's Done channel is
// closed, whichever happens first.
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete.
func (t *RealTimeApi) WithDeadline(ctx context.Context, tm time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, tm)
}

// WithTimeout returns WithDeadline(parent, time.Now().Add(timeout)).
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete:
//
//	func slowOperationWithTimeout(ctx context.Context) (Result, error) {
//		ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
//		defer cancel()  // releases resources if slowOperation completes before timeout elapses
//		return slowOperation(ctx)
//	}
func (t *RealTimeApi) WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

// ///////////////////////////////////////////////////////////////////

var boringFiles []string = []string{
	"timerProvider.go",
	"contextProvider.go",
}

func getInterestingCaller() string {
	for i := 2; ; i += 1 {
		_, file, no, ok := runtime.Caller(i)
		if !ok {
			return "?:?"
		}
		isBoring := false
		for _, boring := range boringFiles {
			if strings.HasSuffix(file, boring) {
				isBoring = true
				break
			}
		}
		if !isBoring {
			return file + ":" + strconv.Itoa(no)
		}
	}
}
