package timeApi

import (
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUselessHelpersFake(t *testing.T) {
	// Test Fake
	startTime := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	_ = WithFakeTime(startTime, func(timeApi *FakeTimeApi) {
		plus5Minutes := startTime.Add(5 * time.Minute)
		minus10Minutes := startTime.Add(-10 * time.Minute)
		until := timeApi.Until(plus5Minutes)
		assert.Equal(t, 5*time.Minute, until)
		since := timeApi.Since(minus10Minutes)
		assert.Equal(t, 10*time.Minute, since)
	})

}

func TestFakeApiStartStopTimer(t *testing.T) {
	now := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	then := now.Add(1 * time.Second)
	timeapi := WithFakeTime(now, func(timeApi *FakeTimeApi) {
		assert.Equal(t, 0, timeApi.TickProducerCount())
		assert.Equal(t, now, timeApi.Now())
		timeApi.IncrementClock(1 * time.Second)
	})
	assert.Equal(t, then, timeapi.Now())
	assert.True(t, timeapi.isStopped)
	AssertEventCount(t, timeapi, 1)

	// make sure we can start time api a second time
	timeapi.Start(now)
	timeapi.Stop()
	assert.Equal(t, now, timeapi.Now())
	assert.True(t, timeapi.isStopped)
	AssertEventCount(t, timeapi, 0)
}

func TestFakeApiAfterFuncAndAfterAndTimer(t *testing.T) {
	now := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	var tickCount int64
	atomic.StoreInt64(&tickCount, 0)
	timeapi := WithFakeTime(now, func(timeApi *FakeTimeApi) {
		timer := timeApi.AfterFunc(1*time.Hour, func() {
			// this executes on another thread
			atomic.AddInt64(&tickCount, 1)
		})
		assert.NotNil(t, timer)
		timerDrained := timer.Stop()
		assert.True(t, timerDrained)
		assert.Equal(t, 1, timeApi.TickProducerCount())

		// timer.Stop() // 2nd stop will produce a panic
		timer.Reset(24 * time.Hour)
		afterC := timeApi.After(24 * time.Hour)
		go func() {
			// this executes on another thread
			<-afterC
			atomic.AddInt64(&tickCount, 1)
		}()
		timer3 := timeApi.NewTimer(30 * time.Hour)
		go func() {
			// this executes on another thread
			<-timer3.C
			atomic.AddInt64(&tickCount, 1)
		}()

		assert.Equal(t, now, timeApi.Now())
		timeApi.IncrementClock(29 * time.Hour)
		timeApi.Sleep(1 * time.Hour)
		assert.Equal(t, now.Add(30*time.Hour), timeApi.Now())
	})
	AssertEventCount(t, timeapi, 12)

	assert.Equal(t, int64(3), atomic.LoadInt64(&tickCount))
	assert.True(t, timeapi.isStopped)
}

func TestFakeApiTickZeroReturnsNil(t *testing.T) {
	_ = WithFakeTime(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC), func(timeApi *FakeTimeApi) {
		assert.Nil(t, timeApi.Tick(0*time.Millisecond))
	})
}

func TestFakeTimeApiOptions(t *testing.T) {
	timeapi := NewFake().
		SetOptions(FakeOptions().WithTesting(t).WithFlushTime(4 * time.Millisecond)).
		Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))
	timeapi.Stop()
	assert.Equal(t, 4*time.Millisecond, timeapi.options.flushTime)
	assert.Equal(t, t, timeapi.options.testingTB)
}

func TestFakeApi(t *testing.T) {
	timeapi := NewFake().Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))

	var itMatchesInterface TimeApi = timeapi
	assert.NotNil(t, itMatchesInterface)

	var tickCount int64
	atomic.StoreInt64(&tickCount, 0)
	var didCheck int64
	atomic.StoreInt64(&didCheck, 0)

	timerAfterFunc := timeapi.AfterFunc(1*time.Hour, func() {
		// this executes on another thread
		atomic.AddInt64(&tickCount, 1)
	})
	assert.NotNil(t, timerAfterFunc)
	timerAfterFuncDrained := timerAfterFunc.Stop()
	assert.True(t, timerAfterFuncDrained)
	assert.Equal(t, int64(0), atomic.LoadInt64(&tickCount))
	timerAfterFunc.Reset(2 * time.Millisecond)

	afterC := timeapi.After(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-afterC
		atomic.AddInt64(&tickCount, 1)
	}()

	timer3 := timeapi.NewTimer(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-timer3.C
		atomic.AddInt64(&tickCount, 1)
	}()

	ticker := timeapi.NewTicker(1 * time.Hour)
	go func() {
		// this executes on another thread
		<-ticker.C
		atomic.AddInt64(&tickCount, 1)
	}()
	ticker.Stop()
	ticker.Reset(2 * time.Millisecond)

	tickC := timeapi.Tick(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-tickC
		atomic.AddInt64(&tickCount, 1)
	}()
	// ticker.Stop() // Truly a monumentally stupid API, leaks by design, should be purged with fire

	expectedCount := int64(5)

	beforeSleepTime := timeapi.Now()
	assert.Equal(t, int64(0), atomic.LoadInt64(&tickCount))
	timeapi.Sleep(2 * time.Millisecond)
	afterSleepTime := timeapi.Now()
	elapsedSleep := afterSleepTime.Sub(beforeSleepTime)
	assert.GreaterOrEqual(t, elapsedSleep, 2*time.Millisecond)
	ticker.Stop()

	// this might be problematic, this is a race condition, theres no guarantee all the other threads have successfully incremented yet
	assert.Equal(t, expectedCount, atomic.LoadInt64(&tickCount))

	beforeGoschedTime := timeapi.Now()
	timeapi.Gosched()
	afterGoschedTime := timeapi.Now()
	elapsedGosched := afterGoschedTime.Sub(beforeGoschedTime)
	assert.GreaterOrEqual(t, elapsedGosched, 45*time.Nanosecond)

	timeapi.Stop()
	events := timeapi.AppendEvents(make([]string, 0, 1024))
	assert.Equal(t, 21, len(events)) // ideally this is 19 here

	// this might be problematic, this is a race condition, theres no guarantee all the other threads have successfully incremented yet
	assert.Equal(t, expectedCount, atomic.LoadInt64(&tickCount))
	AssertEventCount(t, timeapi, 21)

	assert.Equal(t, 1, timeapi.TickProducerCount())

	tickProducerNames := timeapi.AppendTickProducerNames(make([]string, 0, 1024))
	assert.Equal(t, 1, len(tickProducerNames))

	assert.True(t, strings.HasPrefix(tickProducerNames[0], "tp/4/Tick"))
	assert.True(t, strings.HasSuffix(tickProducerNames[0], "(Always leaks - dont use)"))
}

func TestDurationString(t *testing.T) {
	assert.Equal(t, "2h", DurationString(2*time.Hour))
	assert.Equal(t, "2m", DurationString(2*time.Minute))
	assert.Equal(t, "2s", DurationString(2*time.Second))
	assert.Equal(t, "2ms", DurationString(2*time.Millisecond))
	assert.Equal(t, "2us", DurationString(2*time.Microsecond))
	assert.Equal(t, "2ns", DurationString(2*time.Nanosecond))
}

func BenchmarkGosched(b *testing.B) {
	// var amountToSleep time.Duration
	for i := 0; i < b.N; i++ {
		//realNow := time.Now()
		runtime.Gosched()
		//realEnd := time.Now()
		//amountToSleep += realEnd.Sub(realNow)
	}
	//b.Logf("Gosched wall clock elapsed: %v", int(amountToSleep)/b.N)
}
