package timeApi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContextWithDeadline(t *testing.T) {
	startTime := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	plus5Minutes := startTime.Add(5 * time.Minute)

	t.Run("panics on nil parent", func(t *testing.T) {
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			assert.Panics(t, func() {
				timeapi.WithDeadline(nil, timeapi.Now())
			})
		})
	})

	t.Run("can get values", func(t *testing.T) {
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			ctx, _ := timeapi.WithDeadline(
				context.WithValue(
					context.Background(), "foo", "bar"),
				plus5Minutes)

			assert.Equal(t, "bar", ctx.Value("foo"))
		})
	})

	t.Run("implements stringer", func(t *testing.T) {
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			ctx, _ := timeapi.WithDeadline(context.Background(), plus5Minutes)

			stringer, ok := ctx.(stringer)
			assert.True(t, ok)
			assert.True(t, strings.Contains(stringer.String(), "WithDeadline"))
		})
	})

	t.Run("immediately times out when time is now", func(t *testing.T) {
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			ctx, cancel := timeapi.WithDeadline(context.Background(), timeapi.Now())
			assert.NotNil(t, ctx.Done())
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
			<-ctx.Done()
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
			cancel()
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
		})
	})

	t.Run("times out, and then ignores cancel calls", func(t *testing.T) {
		// Test Fake
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			plus5Minutes := startTime.Add(5 * time.Minute)

			ctx, cancel := timeapi.WithDeadline(context.Background(), plus5Minutes)
			assert.NotNil(t, ctx.Done())
			assert.Nil(t, ctx.Err())

			timeapi.IncrementClock(5 * time.Minute)
			<-ctx.Done()
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
			cancel()
			assert.Equal(t, context.DeadlineExceeded, ctx.Err())
		})
	})

	t.Run("can be canceled, just the once", func(t *testing.T) {
		// Test Fake
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			plus5Minutes := startTime.Add(5 * time.Minute)

			ctx, cancel := timeapi.WithDeadline(context.Background(), plus5Minutes)
			assert.NotNil(t, ctx.Done())
			assert.Nil(t, ctx.Err())

			cancel()
			assert.Equal(t, context.Canceled, ctx.Err())
			<-ctx.Done()
			cancel()
			assert.Equal(t, context.Canceled, ctx.Err())

		})
	})

}

func TestContextWithTimeout(t *testing.T) {
	startTime := time.Date(2009, 11, 17, 20, 34, 58, 0, time.UTC)
	plus5Minutes := startTime.Add(5 * time.Minute)

	t.Run("calculates deadline correctly", func(t *testing.T) {
		_ = WithFakeTime(startTime, func(timeapi *FakeTimeApi) {
			ctx, _ := timeapi.WithTimeout(context.Background(), 5*time.Minute)
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.Equal(t, plus5Minutes, deadline)
		})
	})
}
