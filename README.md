[![GitHub issues](https://img.shields.io/github/issues/Villenny/timeApi-go)](https://github.com/Villenny/timeApi-go/issues)
[![GitHub forks](https://img.shields.io/github/forks/Villenny/timeApi-go)](https://github.com/Villenny/timeApi-go/network)
[![GitHub stars](https://img.shields.io/github/stars/Villenny/timeApi-go)](https://github.com/Villenny/timeApi-go/stargazers)
[![GitHub license](https://img.shields.io/github/license/Villenny/timeApi-go)](https://github.com/Villenny/timeApi-go/blob/main/LICENSE)
![Go](https://github.com/Villenny/timeApi-go/workflows/Go/badge.svg?branch=main)
![Codecov branch](https://img.shields.io/codecov/c/github/villenny/timeApi-go/main)
[![Go Report Card](https://goreportcard.com/badge/github.com/Villenny/timeApi-go)](https://goreportcard.com/report/github.com/Villenny/timeApi-go)
[![Documentation](https://godoc.org/github.com/Villenny/timeApi-go?status.svg)](http://godoc.org/github.com/Villenny/timeApi-go)

# timeApi-go
Non global instance wrapper for go time/context/gosched calls, plus matching fake for tests.
It provides an interface around the standard library's time package so that the application can use the realtime clock
while tests can use the mock clock.

Inspired by https://github.com/benbjohnson/clock it worked great for a while until my project had too many bg threads running.


# Install

```
go get -u github.com/Villenny/timeApi-go
```




# Using the Package

The expected use case:
- just the same as the system time library more or less
```
import "github.com/villenny/timeApi-go"

// timeApi mirrors all the time calls
timeapi := timeApi.New()
startTime := timeapi.Now()
```

- The interface for reference
```
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
```




# Using the test fake

## Using fakeTimeApi:
- In general advancing the fake clock will cause sleeps after any timer event to allow bg threads to process, for a surprisingly long time too, since cloud build systems tend to be oversubscribed
```
import "github.com/villenny/timeApi-go"

// initialize and start the timeApi
timeapi := timeApi.NewFake().Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))

// advance the fake clock
timeapi.AdvanceClock(2 * time.Second)

timeapi.Stop()

AssertEventCount(t, timeapi, 0)

```



## Debugging with the fake time event log
- One of the key features of the fake, is that it allows you to get the serialized list of timer events, including the ability to inject your own events, and then assert the count is what you expect.

- In the event the assertion fails you will get something like this:
```
--- FAIL: TestFakeApi (0.11s)
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:750: AssertEventCount: expected 0, got 21
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    0 0000000000000000 newTimer: tp/0/AfterFunc/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:103 60m
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    1 0000000000000000 Stop tp/0/AfterFunc/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:103 isDrained=true
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    2 0000000000000000 Reset tp/0/AfterFunc/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:103 2ms gotTick=false
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    3 0000000000000000 newTimer: tp/1/After/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:113 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    4 0000000000000000 newTimer: tp/2/Timer/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:120 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    5 0000000000000000 newTicker: tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 60m
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    6 0000000000000000 Stopped: tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 drainCount: 0
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    7 0000000000000000 Stopped: tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 (Wasnt running)
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    8 0000000000000000 Reset tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:    9 0000000000000000 newTicker: tp/4/Tick/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:136 (Always leaks - dont use) 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   10 0000000000000000 +Sleep: 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   11 0000000000000002 | DoTick: tp/4/Tick/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:136 (Always leaks - dont use) - 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   12 0000000000000002 | DoTick: tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 - 0ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   13 0000000000000002 | DoTick: tp/2/Timer/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:120 - 0ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   14 0000000000000002 | DoTick: tp/1/After/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:113 - 0ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   15 0000000000000002 | DoTick: tp/0/AfterFunc/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:103 - 0ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   16 0000000000000002 -Sleep: 2ms
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   17 0000000000000002 Stopped: tp/3/Ticker/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:127 drainCount: 0
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   18 0000000000000002 +Gosched: 60ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   19 0000000000000002 -Gosched: 60ns
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:752:   20 0000000000000002 Leaked Tickproducer: tp/4/Tick/c:/dev/GitHub/Villenny/timeApi-go/fakeTimeApi_test.go:136 (Always leaks - dont use)
    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:755:
            Error Trace:    c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi.go:755
                                        c:\dev\GitHub\Villenny\timeApi-go\fakeTimeApi_test.go:169
            Error:          Not equal:
                            expected: 0
                            actual  : 21
            Test:           TestFakeApi
```

## Injecting your own events
- the fake time allows you to inject your own events into the time fakes event log
- this will greatly help you track down what ran when after more complicated tests with many threads

```
fakeTimeApi, ok := timeapi.(*FakeTimeApi)
if ok {
    fakeTimeApi.AddEvent("    My Thingy Ran!")
}
```

## Sometimes the defaults just aint right
- the fake checks that your channels are drained and such, passing the the testing context b or t, will give you more info if it detects something and panics
- cloud build systems under heavy load can take forever to allow bg threads to all run during tests, sometimes you need to wait longer
- conversely if you have a test that is slow due to all the background sleeping maybe you want to reduce flush time, or eliminate it

```
timeapi := timeApi.NewFake().
    SetOptions(timeApi.FakeOptions().WithTesting(t).WithFlushTime(4 * time.Millisecond)).
    Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))
```

## Monitoring your timer state
- internally the time api stores anything that "ticks" as a TickProducer.

```
timeapi := timeApi.NewFake().Start(time.Now())

assert.Equal(t, 0, timeapi.TickProducerCount())

events := timeapi.AppendEvents(make([]string, 0, 1024))
assert.Equal(t, 0, len(events))

tickProducerNames := timeapi.AppendTickProducerNames(make([]string, 0, 1024))
assert.Equal(t, 0, len(tickProducerNames))
```


# Using the timerProvider convenience wrapper

Using the timerProvider
- this providers an easy way to start/stop a bg thread that calls a worker function, with race safety etc.
- it also injects its update calls into the fake timer event log

```
// init time Api
timeapi := timeApi.New()

// start my timer
timerProvider, _ := NewTimerProvider(timeapi)
var runCount int
const CHECK_INTERVAL = 100 * time.Millisecond
timer := timerProvider.SetInterval(func() { runCount += 1 }, CHECK_INTERVAL)

// this stops the timer cleanly, leaking nothing, plus has a lock internally, so you can assert anything it touched safely in tests
// like the above runCount variable.
timer.Stop()
```

You can also monitor the timer for how many times its ticked, or wait until its done the tick count you desire this can be helpful in tests:
```
timeapi := timeApi.NewFake().
    SetOptions(timeApi.FakeOptions().WithTesting(t).WithFlushTime(0)).
    Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))

timerProvider, _ := NewTimerProvider(timeapi)
var runCount int
const CHECK_INTERVAL = 100 * time.Millisecond
timer := timerProvider.SetInterval(func() { runCount += 1 }, CHECK_INTERVAL)
timeapi.IncrementClock(CHECK_INTERVAL)
timer.WaitUntilCount(1)

assert.Equal(t, timer.Count(), runCount)
timeapi.Stop()
assert.Equal(t, 1, runCount)
```


# Benchmark
- Not really relevant to this module


# Contact

Ryan Haksi [ryan.haksi@gmail.com]

# License

Available under the MIT [License](/LICENSE).
