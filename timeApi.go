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

type TickerAdapter interface {
	Stop()

	// Reset stops a ticker and resets its period to the specified duration.
	// The next tick will arrive after the new period elapses. The duration d
	// must be greater than zero; if not, Reset will panic.
	Reset(d Duration)
}

type Ticker struct {
	C      <-chan Time
	ticker TickerAdapter
	name   string
}

func (t *Ticker) Stop()            { t.ticker.Stop() }
func (t *Ticker) Reset(d Duration) { t.ticker.Reset(d) }

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

type Timer struct {
	C     <-chan Time
	timer TimerAdapter
	name  string
}

func (t *Timer) Stop() bool            { return t.timer.Stop() }
func (t *Timer) Reset(d Duration) bool { return t.timer.Reset(d) }

// Inspired by github.com/benbjohnson/clock
type TimeApi interface {
	After(d time.Duration) <-chan time.Time
	AfterFunc(d time.Duration, f func()) *Timer
	Now() time.Time
	Since(t time.Time) time.Duration
	Until(t time.Time) time.Duration
	Sleep(d time.Duration)
	Gosched()
	Tick(d time.Duration) <-chan time.Time
	Ticker(d time.Duration) *Ticker
	Timer(d time.Duration) *Timer
	WithDeadline(parent context.Context, d time.Time) (context.Context, context.CancelFunc)
	WithTimeout(parent context.Context, t time.Duration) (context.Context, context.CancelFunc)
}

// ///////////////////////////////////////////////////////////////////

type RealTimeApi struct {
	nextId int64
}

// New returns an instance of a real-time clock.
func New() *RealTimeApi {
	return &RealTimeApi{}
}

/* this is REALLY not a great API on the part of go, the lack of stop signals and the resulting leak of resources is unconscionable */

func (t *RealTimeApi) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (t *RealTimeApi) AfterFunc(d time.Duration, f func()) *Timer {
	i := time.AfterFunc(d, f)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Timer{i.C, i, "AfterFunc/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}
func (t *RealTimeApi) Now() time.Time                        { return time.Now() }
func (t *RealTimeApi) Since(tm time.Time) time.Duration      { return time.Since(tm) }
func (t *RealTimeApi) Until(tm time.Time) time.Duration      { return time.Until(tm) }
func (t *RealTimeApi) Sleep(d time.Duration)                 { time.Sleep(d) }     // Note on windows golang maximum sleep resolution is roughly 2ms, any amount < than that will sleep for that long
func (t *RealTimeApi) Gosched()                              { runtime.Gosched() } // Note gosched typically takes about 45-60ns to run on my machine
func (t *RealTimeApi) Tick(d time.Duration) <-chan time.Time { return time.Tick(d) }
func (t *RealTimeApi) Ticker(d time.Duration) *Ticker {
	i := time.NewTicker(d)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Ticker{i.C, i, "Ticker/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}
func (t *RealTimeApi) Timer(d time.Duration) *Timer {
	i := time.NewTimer(d)
	id := atomic.AddInt64(&t.nextId, 1)
	return &Timer{i.C, i, "Timer/" + strconv.Itoa(int(id)) + "/" + getInterestingCaller()}
}

func (t *RealTimeApi) WithDeadline(ctx context.Context, tm time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(ctx, tm)
}
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
