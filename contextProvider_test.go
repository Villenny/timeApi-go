package timeApi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContextWithDeadline(t *testing.T) {

	t.Run("ContextWithDeadline times out, and then ignores cancel calls", func(t *testing.T) {
		// Test Fake
		startTime := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			plus5Minutes := startTime.Add(5 * time.Minute)

			ctx, cancel := timeapi.WithDeadline(context.Background(), plus5Minutes)
			assert.NotNil(t, ctx.Done())
			assert.Nil(t, ctx.Err())

			timeapi.IncrementClock(5 * time.Minute)
			select {
			case <-ctx.Done():
				assert.Equal(t, context.DeadlineExceeded, ctx.Err())
			default:
				t.Error("context is not cancelled when deadline exceeded")
			}

			cancel()
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())

		})
	})

}
