package timeApi

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeOptions struct {
	testingTB testing.TB
	flushTime time.Duration
}

func FakeOptions() fakeOptions {
	return fakeOptions{
		// on heavily loaded shared tenancy cloud build environments I've seen in excess of 15ms for sleep(1ms)
		// this makes sleeping until all bg threads reactivate and finish their queues tricky.
		flushTime: 2 * time.Millisecond,
	}
}

func (t fakeOptions) WithTesting(tb testing.TB) fakeOptions {
	t.testingTB = tb
	return t
}

func (t fakeOptions) WithFlushTime(d time.Duration) fakeOptions {
	t.flushTime = d
	return t
}

// ///////////////////////////////////////////////////////////////////

type TickProducer interface {
	GetNextEventTimeUnsafe() time.Time
	DoTick(d time.Duration)
	IsAliveUnsafe() bool
	GetNameUnsafe() string
}

type FakeTimeApi struct {
	options          fakeOptions
	mutex            *sync.Mutex
	events           []string
	startTime        time.Time
	now              time.Time
	isStopped        bool
	isAdvancingClock bool
	tickProducers    []TickProducer
	nextId           int
}

func reapEndedTickProducers(it *FakeTimeApi) {
	// swap delete all dead entries
	it.mutex.Lock()
	defer it.mutex.Unlock()
	for i := 0; i < len(it.tickProducers); {
		if it.tickProducers[i].IsAliveUnsafe() {
			i += 1
			continue
		}
		lenMinusOne := len(it.tickProducers) - 1
		it.tickProducers[i] = it.tickProducers[lenMinusOne]
		it.tickProducers[lenMinusOne] = nil
		it.tickProducers = it.tickProducers[0:lenMinusOne]
	}
}

func advanceNowAndRunTickProducers(it *FakeTimeApi, d time.Duration) {
	currentTime := it.Now()

	finalTime := currentTime.Add(d)
	tickCount := 0
	tickedSomething := true
	ticked := make([]TickProducer, 0, 32)
	for tickedSomething {
		var itemToTick TickProducer
		minEventTime := finalTime
		tickedSomething = false
		var elapsedTime time.Duration

		func() {
			it.mutex.Lock()
			defer it.mutex.Unlock()

			// for each running timer, find the next event time, advance that timer to that event time
			for _, item := range it.tickProducers {
				if !item.IsAliveUnsafe() {
					continue
				}
				eventTime := item.GetNextEventTimeUnsafe()
				if eventTime.After(minEventTime) {
					continue
				}
				itemToTick = item
				minEventTime = eventTime
			}
			elapsedTime = minEventTime.Sub(currentTime)

			it.now = it.now.Add(elapsedTime)
			if itemToTick != nil {
				ticked = append(ticked, itemToTick)
				it.events = append(it.events, fmt.Sprintf("%016d | DoTick: %v - %v", it.now.Sub(it.startTime)/time.Millisecond, itemToTick.GetNameUnsafe(), DurationString(elapsedTime)))
			}
		}()

		// tick the item but give up the lock first, so it can get it, and release it as necessary to drive bg threads listening to timers
		if itemToTick != nil {
			tickedSomething = true
			itemToTick.DoTick(elapsedTime)
			tickCount += 1
		}
		currentTime = currentTime.Add(elapsedTime)
	}
}

func (t *FakeTimeApi) getInterestingCaller() string {
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

// New returns an instance of a real-time clock.
// Optionally you can have a bg thread to advance the clock 1ms every 500ms or something so time is always passing at least a little
func NewFake(opts ...fakeOptions) *FakeTimeApi {
	o := FakeOptions()
	if len(opts) > 1 {
		panic("Dont be crazy")
	}
	if len(opts) > 0 {
		o = opts[0]
	}

	fake := &FakeTimeApi{
		mutex:     &sync.Mutex{},
		options:   o,
		isStopped: true,
	}
	return fake
}

func (t *FakeTimeApi) SetOptions(opts fakeOptions) *FakeTimeApi {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.isStopped {
		panic("Timers already started")
	}
	t.options = opts
	return t
}

func (t *FakeTimeApi) Start(tm time.Time) *FakeTimeApi {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.isStopped {
		panic("Timers already started")
	}

	t.events = make([]string, 0, 512)
	t.startTime = tm
	t.now = tm
	t.isStopped = false
	if len(t.tickProducers) > 0 {
		t.logEventsToTestingUnsafe()
		panic("orphaned tick producers detected")
	}
	t.tickProducers = t.tickProducers[:0]
	return t
}

func (t *FakeTimeApi) Stop() *FakeTimeApi {
	time.Sleep(t.options.flushTime)

	reapEndedTickProducers(t)

	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.isStopped = true

	// unfortunately using the Tick() function leaks a tick producer by design so we cant panic on every leak
	importantLeak := false
	for _, i := range t.tickProducers {
		t.events = append(t.events, fmt.Sprintf("%016d Leaked Tickproducer: %v", t.now.Sub(t.startTime)/time.Millisecond, i.GetNameUnsafe()))
	}

	if importantLeak {
		t.logEventsToTestingUnsafe()
	}

	return t
}

func WithFakeTime(startTime time.Time, fn func(timeApi *FakeTimeApi)) *FakeTimeApi {
	fakeTimeApi := NewFake().Start(startTime)
	fn(fakeTimeApi)
	fakeTimeApi.Stop()
	return fakeTimeApi
}

func (t *FakeTimeApi) AddEvent(event string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.events = append(t.events, fmt.Sprintf("%016d %v", t.now.Sub(t.startTime)/time.Millisecond, event))
}

func (t *FakeTimeApi) incrementClock(d time.Duration, eventName string) *FakeTimeApi {
	if d == 0 {
		return t
	}

	isEarlyOut := false
	isWaitingToAdvanceClock := true
	for {
		func() {
			t.mutex.Lock()
			defer t.mutex.Unlock()
			if t.isStopped {
				panic("Timers arent started, Try again")
			}

			if len(t.tickProducers) == 0 {
				t.now = t.now.Add(d)
				t.events = append(t.events, fmt.Sprintf("%016d %v: %v (no producers)", t.now.Sub(t.startTime)/time.Millisecond, eventName, DurationString(d)))
				isEarlyOut = true
				isWaitingToAdvanceClock = false
				return
			}

			if !t.isAdvancingClock == true {
				t.isAdvancingClock = true
				isWaitingToAdvanceClock = false
				if !isEarlyOut {
					t.events = append(t.events, fmt.Sprintf("%016d +%v: %v", t.now.Sub(t.startTime)/time.Millisecond, eventName, DurationString(d)))
				}
				return
			}
		}()

		if isWaitingToAdvanceClock == true {
			time.Sleep(1 * time.Millisecond)
			continue
		}
		break
	}

	if isEarlyOut {
		return t
	}

	advanceNowAndRunTickProducers(t, d)

	// reap dead items
	reapEndedTickProducers(t)

	func() {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		t.events = append(t.events, fmt.Sprintf("%016d -%v: %v", t.now.Sub(t.startTime)/time.Millisecond, eventName, DurationString(d)))
		t.isAdvancingClock = false
	}()

	// If this is emulating gosched we dont want to sleep, although we cant guarantee we didnt above as part of synchronization around advancing the clock, assuming the gosched did that:
	if eventName != "Gosched" {
		time.Sleep(t.options.flushTime) // given some time to any background threads to let them get caught up
	}
	return t
}

func (t *FakeTimeApi) IncrementClock(d time.Duration) *FakeTimeApi {
	t.incrementClock(d, "IncrementClock")
	return t
}

func (t *FakeTimeApi) After(d time.Duration) <-chan time.Time {
	return t.newTimer(d, nil, "After/"+t.getInterestingCaller()).C
}

func (t *FakeTimeApi) AfterFunc(d time.Duration, f func()) *Timer {
	i := t.newTimer(d, f, "AfterFunc/"+t.getInterestingCaller())
	return &Timer{i.C, i, i.name}
}

func (t *FakeTimeApi) Now() time.Time {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.now
}

func (t *FakeTimeApi) Since(tm time.Time) time.Duration {
	return t.Now().Sub(tm)
}

func (t *FakeTimeApi) Until(tm time.Time) time.Duration {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return tm.Sub(t.now)
}

// WARNING! If you sleep concurrently the clock will advance non-deterministically, in the event you sleep less than 2ms, the clock will advance 2ms
// to match real world sleep behavior on windows (ie you sleep at least the requested amount, bounded by clock granularity)
func (t *FakeTimeApi) Sleep(d time.Duration) {
	//time.Sleep(1 * time.Millisecond)

	now := t.Now()
	if d < 2*time.Millisecond {
		// on windows minimum sleep time can be roughly 2ms
		d = 2 * time.Millisecond
	}

	until := now.Add(d)

	// <-t.After(d)
	amountToSleep := until.Sub(t.Now())
	if amountToSleep > 0 {
		t.incrementClock(amountToSleep, "Sleep")
	}
	return
}

func (t *FakeTimeApi) Gosched() {
	//realNow := time.Now()
	runtime.Gosched() // gosched typically takes about 45-60ns to run on my machine, but sometimes 0ns presumably depending on core task queue size
	//realEnd := time.Now()
	//amountToSleep := realEnd.Sub(realNow)
	amountToSleep := 60 * time.Nanosecond
	if amountToSleep > 0 {
		t.incrementClock(amountToSleep, "Gosched")
	}
	return
}

func (t *FakeTimeApi) Tick(d time.Duration) <-chan time.Time {
	if d <= 0 {
		return nil
	}
	return t.newTicker(d, "Tick/"+t.getInterestingCaller()+" (Always leaks - dont use)").C
}

func (t *FakeTimeApi) Ticker(d time.Duration) *Ticker {
	i := t.newTicker(d, "Ticker/"+t.getInterestingCaller())
	return &Ticker{i.C, i, i.name}
}

func (t *FakeTimeApi) Timer(d time.Duration) *Timer {
	i := t.newTimer(d, nil, "Timer/"+t.getInterestingCaller())
	return &Timer{i.C, i, i.name}
}

func (t *FakeTimeApi) TickProducerCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return len(t.tickProducers)
}

func (t *FakeTimeApi) AppendEvents(events []string) []string {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return append(events, t.events...)
}

func (t *FakeTimeApi) logEventsToTestingUnsafe() {
	tb := t.options.testingTB
	if tb == nil {
		return
	}
	events := t.events
	tb.Logf("FakeTimeApi Events: count=%v", len(events))
	for i, e := range events {
		tb.Logf("  %2d %v", i, e)
	}
}

// ///////////////////////////////////////////////////////////////////

type FakeTicker struct {
	fakeTimeApi *FakeTimeApi
	name        string
	when        time.Time
	d           time.Duration
	C           <-chan Time // The channel on which the ticks are delivered.
	c           chan Time   // The channel on which the ticks are delivered.
	tickCount   int64
	isStopped   bool
}

func (t *FakeTimeApi) newTicker(d time.Duration, typeName string) *FakeTicker {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if d <= 0 {
		panic("cant reset to a duration <= 0")
	}

	c := make(chan Time, 1)
	fake := &FakeTicker{
		fakeTimeApi: t,
		name:        "tp/" + strconv.Itoa(t.nextId) + "/" + typeName,
		when:        t.now,
		d:           d,
		C:           c,
		c:           c,
		tickCount:   0,
		isStopped:   false,
	}

	t.tickProducers = append(t.tickProducers, fake)
	t.nextId++
	t.events = append(t.events, fmt.Sprintf("%016d newTicker: %v %v", t.now.Sub(t.startTime)/time.Millisecond, fake.name, DurationString(d)))
	return fake
}

// NewTicker returns a new Ticker containing a channel that will send
// the current time on the channel after each tick. The period of the
// ticks is specified by the duration argument. The ticker will adjust
// the time interval or drop ticks to make up for slow receivers.
// The duration d must be greater than zero; if not, NewTicker will
// panic. Stop the ticker to release associated resources.

// Stop turns off a ticker. After Stop, no more ticks will be sent.
// Stop does not close the channel, to prevent a concurrent goroutine
// reading from the channel from seeing an erroneous "tick".

func (t *FakeTicker) Stop() {
	alreadyStopped := false
	func() {
		t.fakeTimeApi.mutex.Lock()
		defer t.fakeTimeApi.mutex.Unlock()
		if t.isStopped {
			t.fakeTimeApi.events = append(t.fakeTimeApi.events, fmt.Sprintf("%016d Stopped: %v (Wasnt running)", t.fakeTimeApi.now.Sub(t.fakeTimeApi.startTime)/time.Millisecond, t.name))
			alreadyStopped = true
		}
		t.isStopped = true
	}()
	if alreadyStopped {
		return // multiple calls to stop the same ticker is sadly valid in the real API
	}

	// wait for channel to drain
	i := 0
	iMax := 10
	for ; i < iMax && len(t.c) > 0; i += 1 {
		time.Sleep(1 * time.Millisecond)
	}
	func() {
		t.fakeTimeApi.mutex.Lock()
		defer t.fakeTimeApi.mutex.Unlock()

		if len(t.c) > 0 {
			t.fakeTimeApi.logEventsToTestingUnsafe()
			panic("Ticker channel didnt drain during/after stop() call")
		}

		t.fakeTimeApi.events = append(t.fakeTimeApi.events, fmt.Sprintf("%016d Stopped: %v drainCount: %v", t.fakeTimeApi.now.Sub(t.fakeTimeApi.startTime)/time.Millisecond, t.name, i))
	}()
}

// Reset stops a ticker and resets its period to the specified duration.
// The next tick will arrive after the new period elapses. The duration d
// must be greater than zero; if not, Reset will panic.
func (t *FakeTicker) Reset(d Duration) {
	if d <= 0 {
		panic("cant reset to a duration <= 0")
	}

	t.Stop()

	t.fakeTimeApi.mutex.Lock()
	defer t.fakeTimeApi.mutex.Unlock()

	t.when = t.fakeTimeApi.now
	t.d = d
	t.tickCount = 0
	t.isStopped = false

	t.fakeTimeApi.events = append(t.fakeTimeApi.events, fmt.Sprintf("%016d Reset %v %v", t.fakeTimeApi.now.Sub(t.fakeTimeApi.startTime)/time.Millisecond, t.name, DurationString(d)))
}

func (t *FakeTicker) GetNextEventTimeUnsafe() time.Time {
	if t.isStopped {
		return MaxTime
	}

	nextTickCount := t.tickCount + 1
	return t.when.Add(time.Duration(nextTickCount) * t.d)
}

func (t *FakeTicker) DoTick(d time.Duration) {
	// wait for channel to drain
	i := 0
	iMax := 10
	for ; i < iMax && len(t.c) > 0; i += 1 {
		time.Sleep(1 * time.Millisecond)
	}

	var now time.Time
	func() {
		t.fakeTimeApi.mutex.Lock()
		defer t.fakeTimeApi.mutex.Unlock()

		if t.isStopped {
			return
		}
		if len(t.c) > 0 {
			// tickers will not produce overlapping ticks, and will silently drop ticks to catch up
			return
		}
		nextTickCount := t.tickCount + 1
		now = t.when.Add(time.Duration(nextTickCount) * t.d)
		t.tickCount += 1
	}()
	t.c <- now

	// wait for channel to drain
	i = 0
	iMax = 10
	for ; i < iMax && len(t.c) > 0; i += 1 {
		time.Sleep(1 * time.Millisecond)
	}
	time.Sleep(t.fakeTimeApi.options.flushTime)
}

func (t *FakeTicker) IsAliveUnsafe() bool {
	// this is problematic, because it cant be accessed while the timer lock is held else we can have deadlocks
	if len(t.C) > 0 {
		return true
	}
	return t.isStopped == false
}

func (t *FakeTicker) GetNameUnsafe() string {
	return t.name
}

// ///////////////////////////////////////////////////////////////////

type FakeTimer struct {
	fakeTimeApi *FakeTimeApi
	name        string
	when        time.Time
	d           time.Duration
	C           <-chan Time // The channel on which the ticks are delivered.
	c           chan Time   // The channel on which the ticks are delivered.
	fn          func()
	gotTick     bool
	isStopped   bool
}

func (t *FakeTimeApi) newTimer(d time.Duration, fn func(), typeName string) *FakeTimer {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	var c chan Time
	if fn == nil {
		c = make(chan Time, 1)
	}
	fake := &FakeTimer{
		fakeTimeApi: t,
		name:        "tp/" + strconv.Itoa(t.nextId) + "/" + typeName,
		fn:          fn,
		when:        t.now,
		d:           d,
		C:           c,
		c:           c,
		gotTick:     false,
		isStopped:   false,
	}

	t.tickProducers = append(t.tickProducers, fake)
	t.nextId++
	t.events = append(t.events, fmt.Sprintf("%016d newTimer: %v %v", t.now.Sub(t.startTime)/time.Millisecond, fake.name, DurationString(d)))
	return fake
}

// Reset changes the timer to expire after duration d.
// It returns true if the timer had been active, false if the timer had
// expired or been stopped.
//
// For a Timer created with NewTimer, Reset should be invoked only on
// stopped or expired timers with drained channels.
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
//
// This should not be done concurrent to other receives from the Timer's
// channel.
//
// Note that it is not possible to use Reset's return value correctly, as there
// is a race condition between draining the channel and the new timer expiring.
// Reset should always be invoked on stopped or expired channels, as described above.
// The return value exists to preserve compatibility with existing programs.
//
// For a Timer created with AfterFunc(d, f), Reset either reschedules
// when f will run, in which case Reset returns true, or schedules f
// to run again, in which case it returns false.
// When Reset returns false, Reset neither waits for the prior f to
// complete before returning nor does it guarantee that the subsequent
// goroutine running f does not run concurrently with the prior
// one. If the caller needs to know whether the prior execution of
// f is completed, it must coordinate with f explicitly.

func (t *FakeTimer) Stop() bool {
	t.fakeTimeApi.mutex.Lock()
	defer t.fakeTimeApi.mutex.Unlock()

	if t.isStopped {
		panic("Just stopped a timer thats already stopped")
	}

	t.isStopped = true
	isDrained := len(t.c) == 0

	t.fakeTimeApi.events = append(t.fakeTimeApi.events, fmt.Sprintf("%016d Stop %v isDrained=%v", t.fakeTimeApi.now.Sub(t.fakeTimeApi.startTime)/time.Millisecond, t.name, isDrained))
	return isDrained
}

func (t *FakeTimer) Reset(d Duration) bool {
	t.fakeTimeApi.mutex.Lock()
	defer t.fakeTimeApi.mutex.Unlock()

	if !t.isStopped {
		panic("cant reset a timer which is still running")
	}

	if len(t.C) > 0 {
		panic("unread value in the timer, cant call Reset on a timer that hasnt been stopped and drained")
	}

	gotTick := t.gotTick

	t.when = t.fakeTimeApi.now
	t.d = d
	t.gotTick = false
	t.isStopped = false

	t.fakeTimeApi.events = append(t.fakeTimeApi.events, fmt.Sprintf("%016d Reset %v %v gotTick=%v", t.fakeTimeApi.now.Sub(t.fakeTimeApi.startTime)/time.Millisecond, t.name, DurationString(d), gotTick))
	return gotTick
}

func (t *FakeTimer) GetNextEventTimeUnsafe() time.Time {
	if t.isStopped {
		return MaxTime
	}
	return t.when.Add(t.d)
}

func (t *FakeTimer) DoTick(d time.Duration) {
	// wait for channel to drain
	i := 0
	iMax := 10
	for ; i < iMax && len(t.c) > 0; i += 1 {
		time.Sleep(1 * time.Millisecond)
	}

	var now time.Time
	var fn func()
	func() {
		t.fakeTimeApi.mutex.Lock()
		defer t.fakeTimeApi.mutex.Unlock()

		if t.isStopped {
			return
		}
		t.isStopped = true
		now = t.when.Add(d)
		fn = t.fn
	}()

	if fn != nil {
		fn()
	} else {
		// Hmm this can block and wait for the channel to drain, unlike a real ticker
		//if len(t.c) == 0 {
		t.c <- now
		//}

		// wait for channel to drain
		i = 0
		iMax = 10
		for ; i < iMax && len(t.c) > 0; i += 1 {
			time.Sleep(1 * time.Millisecond)
		}
		time.Sleep(t.fakeTimeApi.options.flushTime)
	}
}

func (t *FakeTimer) IsAliveUnsafe() bool {
	if len(t.C) > 0 {
		return true
	}
	return t.isStopped == false
}

func (t *FakeTimer) GetNameUnsafe() string {
	return t.name
}

// ///////////////////////////////////////////////////////////////////

func DurationString(d time.Duration) string {
	if d >= 2*time.Hour {
		return fmt.Sprintf("%vh", float64(d)/float64(time.Hour))
	}
	if d >= 2*time.Minute {
		return fmt.Sprintf("%vm", float64(d)/float64(time.Minute))
	}
	if d >= 2*time.Second {
		return fmt.Sprintf("%vs", float64(d)/float64(time.Second))
	}
	if d >= 2*time.Millisecond {
		return fmt.Sprintf("%vms", float64(d)/float64(time.Millisecond))
	}
	if d >= 2*time.Microsecond {
		return fmt.Sprintf("%vus", float64(d)/float64(time.Microsecond))
	}
	return fmt.Sprintf("%vns", float64(d)/float64(time.Nanosecond))
}

func AssertEventCount(tb testing.TB, timeApi *FakeTimeApi, count int) {
	events := timeApi.AppendEvents(make([]string, 0, 1024))
	if len(events) != count {
		tb.Logf("AssertEventCount: expected %v, got %v", count, len(events))
		for i, e := range events {
			tb.Logf("  %2d %v", i, e)
		}
	}
	assert.Equal(tb, count, len(events))
}
