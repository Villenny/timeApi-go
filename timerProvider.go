package timeApi

import (
	"strconv"
	"sync"
	"time"
)

// Convenience wrapper for making tickers. Hmm, should probably have called it a TickerFuncProvider
type TimerProvider struct {
	timeApi TimeApi
}

func NewTimerProvider(timeApi TimeApi) (*TimerProvider, error) {
	return &TimerProvider{
		timeApi,
	}, nil
}

// IntervalFuncTicker, should probably be called a TickerFunc
type IntervalFuncTicker struct {
	done       chan bool
	ticker     *Ticker
	fn         func(tm time.Time)
	mutex      *sync.Mutex
	gotDone    bool
	isRunning  bool
	isStopping bool
	count      int
	timeApi    TimeApi
	callerName string
	started    time.Time
}

func wrapperFn(it *IntervalFuncTicker) {
	for !it.gotDone {
		select {
		case item, ok := <-it.ticker.C:
			if item.Unix() == 0 {
				panic("TIME WAS ZERO?!?")
			}
			if ok == false {
				panic("TIMER CHANNELS NEVER CLOSE?!?")
			}

			fakeTimeApi, ok := it.timeApi.(*FakeTimeApi)
			if ok {
				fakeTimeApi.AddEvent("    IntervalTimer: " + it.ticker.name + " " + strconv.Itoa(int(item.Sub(it.started)/time.Millisecond)))
			}

			// terminate goroutine on timer stop
			func() {
				it.mutex.Lock()
				defer it.mutex.Unlock()
				it.fn(item)
				it.count += 1
			}()

		case item, ok := <-it.done:
			if !item {
				panic("this should never be false")
			}

			if item || !ok {
				// terminate goroutine on timer stop
				func() {
					it.mutex.Lock()
					defer it.mutex.Unlock()
					it.gotDone = true
				}()
				return
			}
		}
	}
}

// creates a ticker, and applies a mutex so that the func(tm time.Time) you pass will be invoked every heart beat
// it can only run one at time (it will serialize around its internal mutex)
// stopping it is likewise safe via mutex
func (t *TimerProvider) SetInterval(fn func(tm time.Time), interval time.Duration) *IntervalFuncTicker {
	ticker := t.timeApi.NewTicker(interval)
	done := make(chan bool, 0)

	intervalFuncTicker := &IntervalFuncTicker{
		ticker:    ticker,
		fn:        fn,
		done:      done,
		mutex:     &sync.Mutex{},
		isRunning: true,
		gotDone:   false,
		timeApi:   t.timeApi,
		started:   t.timeApi.Now(),
	}

	go wrapperFn(intervalFuncTicker)

	return intervalFuncTicker
}

// Stop the ticker func from executing in a race safe way.
func (t *IntervalFuncTicker) ClearInterval() *IntervalFuncTicker {
	func() {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		if t.ticker == nil {
			panic("WTF - tried to stop a bogus timer")
		}
		if t.ticker.C == nil {
			panic("WTF - tried to stop a bogus timer")
		}
		if !t.isRunning {
			panic("WTF - tried to stop a timer thats not running")
		}
		if t.gotDone {
			panic("WTF - tried to stop a timer thats stopping")
		}
		if t.isStopping {
			panic("WTF - tried to stop a tmer thats stopping concurrently")
		}
		t.isStopping = true
	}()

	// wait for channel to drain
	i := 0
	iMax := 20
	for ; i < iMax && len(t.ticker.C) > 0; i += 1 {
		t.timeApi.Sleep(1 * time.Millisecond)
	}
	if len(t.ticker.C) > 0 {
		panic("Ticker channel didnt drain during/after ClearInterval() call")
	}
	t.timeApi.Sleep(1 * time.Millisecond)

	func() {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		t.ticker.Stop() // ugh, for some reason timers dont close their channel
		t.done <- true
		close(t.done)
	}()

	isDone := false
	for {
		isDone = func() bool {
			t.mutex.Lock()
			defer t.mutex.Unlock()
			return t.gotDone
		}()
		if isDone {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	t.ticker = nil
	t.fn = nil
	t.isRunning = false
	t.isStopping = false
	return nil
}

// get the number of times the callback function has been invoked
func (t *IntervalFuncTicker) Count() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.count
}

// spin with timeapi.Sleep until the invocation count is as specified
// returns the # of 1ms sleeps it waited.
func (t *IntervalFuncTicker) WaitUntilCount(count int) (sleepCount int) {
	sleepCount = 0
	for count > t.Count() {
		sleepCount += 1
		t.timeApi.Sleep(1 * time.Millisecond)
	}
	return sleepCount
}
