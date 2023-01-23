package timeApi

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUselessHelpersReal(t *testing.T) {
	// Test Real
	timeapi := New()
	startTime := timeapi.Now()
	plus5Minutes := startTime.Add(5 * time.Minute)
	minus10Minutes := startTime.Add(-10 * time.Minute)
	until := timeapi.Until(plus5Minutes)
	assert.LessOrEqual(t, 5*time.Minute-1*time.Millisecond, until)
	assert.GreaterOrEqual(t, 5*time.Minute, until)

	since := timeapi.Since(minus10Minutes)
	assert.LessOrEqual(t, 10*time.Minute, since)
	assert.GreaterOrEqual(t, 10*time.Minute+1*time.Millisecond, since)
}

func TestRealApi(t *testing.T) {
	timeapi := New()

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
	timerAfterFunc.Reset(2 * time.Millisecond)

	afterC := timeapi.After(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-afterC
		atomic.AddInt64(&tickCount, 1)
	}()

	timer3 := timeapi.Timer(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-timer3.C
		atomic.AddInt64(&tickCount, 1)
	}()

	ticker := timeapi.Ticker(1 * time.Hour)
	go func() {
		// this executes on another thread
		<-ticker.C
		atomic.AddInt64(&tickCount, 1)
	}()
	ticker.Stop()
	ticker.Stop()
	ticker.Reset(2 * time.Millisecond)

	tickC := timeapi.Tick(2 * time.Millisecond)
	go func() {
		// this executes on another thread
		<-tickC
		atomic.AddInt64(&tickCount, 1)
	}()
	// ticker.Stop() // Truly a monumentally stupid API, leaks by design, should be purged with fire

	beforeSleepTime := timeapi.Now()
	timeapi.Sleep(2 * time.Millisecond)
	afterSleepTime := timeapi.Now()
	elapsed := afterSleepTime.Sub(beforeSleepTime)

	// with race detection can make this very very slow, mostly the read the time part
	// on heavily loaded shared tenancy cloud build environments I've seen in excess of 15ms
	assert.LessOrEqual(t, elapsed, 20*time.Millisecond)
	assert.GreaterOrEqual(t, elapsed, 2*time.Millisecond)
	ticker.Stop()

	expectedCount := int64(5)
	for i := 0; i < 20 && atomic.LoadInt64(&tickCount) < expectedCount; i += 1 {
		timeapi.Sleep(1 * time.Millisecond)
	}
	// this might be problematic, this is a race condition, theres no guarantee all the other threads have successfully incremented yet
	// although after 10ms, you'd hope so
	assert.Equal(t, expectedCount, atomic.LoadInt64(&tickCount))
}
