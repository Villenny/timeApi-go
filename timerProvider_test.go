package timeApi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStartStopTimer(t *testing.T) {

	now := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	timeApi_ := NewFake().Start(now)

	timerProvider, _ := NewTimerProvider(timeApi_)
	var runCount int
	const CHECK_INTERVAL = 100 * time.Millisecond
	timer := timerProvider.SetInterval(func() { runCount += 1 }, CHECK_INTERVAL)
	assert.Equal(t, 0, runCount)
	assert.Equal(t, timer.Count(), runCount)

	timer.WaitUntilCount(0)
	timeApi_.IncrementClock(CHECK_INTERVAL)
	timer.WaitUntilCount(0)
	timer.WaitUntilCount(1)
	timer.ClearInterval()
	assert.Equal(t, 1, runCount)
	assert.Equal(t, timer.Count(), runCount)

	timeApi_.IncrementClock(CHECK_INTERVAL)
	timeApi_.Stop()
	assert.Equal(t, 1, runCount)
	assert.Equal(t, timer.Count(), runCount)
}
