package timeApi

import (
	"runtime"
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
	timeApi := WithFakeTime(now, func(timeApi *FakeTimeApi) {
		assert.Equal(t, 0, timeApi.TickProducerCount())
		assert.Equal(t, now, timeApi.Now())
		timeApi.IncrementClock(1 * time.Second)
	})
	assert.Equal(t, then, timeApi.Now())
	assert.True(t, timeApi.isStopped)
	AssertEventCount(t, timeApi, 1)

	// make sure we can start time api a second time
	timeApi.Start(now)
	timeApi.Stop()
	assert.Equal(t, now, timeApi.Now())
	assert.True(t, timeApi.isStopped)
	AssertEventCount(t, timeApi, 0)
}

func TestFakeApiAfterFuncAndAfterAndTimer(t *testing.T) {
	now := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	var tickCount int64
	atomic.StoreInt64(&tickCount, 0)
	timeApi := WithFakeTime(now, func(timeApi *FakeTimeApi) {
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
		timer3 := timeApi.Timer(30 * time.Hour)
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
	AssertEventCount(t, timeApi, 12)

	assert.Equal(t, int64(3), atomic.LoadInt64(&tickCount))
	assert.True(t, timeApi.isStopped)
}

func TestFakeApiTickZeroReturnsNil(t *testing.T) {
	_ = WithFakeTime(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC), func(timeApi *FakeTimeApi) {
		assert.Nil(t, timeApi.Tick(0*time.Millisecond))
	})
}

func TestFakeApi(t *testing.T) {
	timeApi := NewFake().Start(time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC))

	var itMatchesInterface TimeApi = timeApi
	assert.NotNil(t, itMatchesInterface)

	var tickCount int64
	atomic.StoreInt64(&tickCount, 0)
	var didCheck int64
	atomic.StoreInt64(&didCheck, 0)

	timerAfterFunc := timeApi.AfterFunc(1*time.Hour, func() {
		// this executes on another thread
		atomic.AddInt64(&tickCount, 1)
	})
	assert.NotNil(t, timerAfterFunc)
	timerAfterFuncDrained := timerAfterFunc.Stop()
	assert.True(t, timerAfterFuncDrained)
	assert.Equal(t, int64(0), atomic.LoadInt64(&tickCount))
	timerAfterFunc.Reset(2 * time.Millisecond)

	afterC := timeApi.After(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-afterC
		atomic.AddInt64(&tickCount, 1)
	}()

	timer3 := timeApi.Timer(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-timer3.C
		atomic.AddInt64(&tickCount, 1)
	}()

	ticker := timeApi.Ticker(1 * time.Hour)
	go func() {
		// this executes on another thread
		<-ticker.C
		atomic.AddInt64(&tickCount, 1)
	}()
	ticker.Stop()
	ticker.Reset(2 * time.Millisecond)

	tickC := timeApi.Tick(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-tickC
		atomic.AddInt64(&tickCount, 1)
	}()
	// ticker.Stop() // Truly a monumentally stupid API, leaks by design, should be purged with fire

	expectedCount := int64(5)

	beforeSleepTime := timeApi.Now()
	assert.Equal(t, int64(0), atomic.LoadInt64(&tickCount))
	timeApi.Sleep(2 * time.Millisecond)
	afterSleepTime := timeApi.Now()
	elapsedSleep := afterSleepTime.Sub(beforeSleepTime)
	assert.GreaterOrEqual(t, elapsedSleep, 2*time.Millisecond)
	ticker.Stop()

	// this might be problematic, this is a race condition, theres no guarantee all the other threads have successfully incremented yet
	assert.Equal(t, expectedCount, atomic.LoadInt64(&tickCount))

	beforeGoschedTime := timeApi.Now()
	timeApi.Gosched()
	afterGoschedTime := timeApi.Now()
	elapsedGosched := afterGoschedTime.Sub(beforeGoschedTime)
	assert.GreaterOrEqual(t, elapsedGosched, 45*time.Nanosecond)

	timeApi.Stop()
	events := timeApi.AppendEvents(make([]string, 0, 1024))
	assert.Equal(t, 21, len(events)) // ideally this is 19 here

	// this might be problematic, this is a race condition, theres no guarantee all the other threads have successfully incremented yet
	assert.Equal(t, expectedCount, atomic.LoadInt64(&tickCount))
	AssertEventCount(t, timeApi, 0)
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
