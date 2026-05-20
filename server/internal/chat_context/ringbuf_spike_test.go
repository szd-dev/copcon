// Spike test for github.com/golang-cz/ringbuf under Go 1.26.
//
// Run: go test ./internal/chat_context/... -run Spike -race -v
//
// This file validates core ringbuf patterns needed for ChatContext:
//   - Fan-out: single writer → many subscribers
//   - Seek replay: StartBehind / SeekAfter
//   - Slow subscriber: MaxLag enforcement and ErrTooSlow

package chat_context

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/golang-cz/ringbuf"
	"github.com/stretchr/testify/require"
)

// TestRingbufFanOut validates single-writer multi-reader fan-out.
// 10 subscribers each receive all 500 events with zero data races.
func TestRingbufSpikeFanOut(t *testing.T) {
	const numSubs = 10
	const numEvents = 500

	ctx := context.Background()
	rb := ringbuf.New[int](1000)

	// Create all subscribers BEFORE the writer starts, so they all start at position 0.
	subs := make([]*ringbuf.Subscriber[int], numSubs)
	for i := range numSubs {
		subs[i] = rb.Subscribe(ctx, &ringbuf.SubscribeOpts{
			Name: fmt.Sprintf("fanout-%d", i),
		})
	}

	// Launch concurrent reader goroutines.
	var wg sync.WaitGroup
	received := make([][]int, numSubs)

	for i := range numSubs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for v := range subs[idx].Iter() {
				received[idx] = append(received[idx], v)
			}
		}(i)
	}

	// Single writer — Write() is NOT concurrent safe, must be called from one goroutine.
	for i := range numEvents {
		rb.Write(i)
	}
	rb.Close()

	wg.Wait()

	// Each subscriber received every event in order.
	for i := range numSubs {
		require.Lenf(t, received[i], numEvents, "subscriber %d should receive all events", i)
		for j := range numEvents {
			require.Equalf(t, j, received[i][j], "subscriber %d event %d mismatch", i, j)
		}
	}
}

// TestRingbufSeekReplay validates StartBehind replay of historical data.
// After writing 100 events, a subscriber with StartBehind=80 should
// start at position 20 and receive exactly 80 events (indices 20-99).
func TestRingbufSpikeSeekReplay(t *testing.T) {
	ctx := context.Background()
	rb := ringbuf.New[int](200)

	// Write 100 events (writePos = 100).
	for i := range 100 {
		rb.Write(i)
	}

	// Subscribe starting 80 items behind writePos (= position 20).
	sub := rb.Subscribe(ctx, &ringbuf.SubscribeOpts{
		Name:        "seek-replay",
		StartBehind: 80,
		MaxLag:      100,
	})

	rb.Close()

	var received []int
	for v := range sub.Iter() {
		received = append(received, v)
	}

	require.Lenf(t, received, 80, "should receive exactly 80 events")
	require.Equalf(t, 20, received[0], "first event should be index 20")
	require.Equalf(t, 99, received[len(received)-1], "last event should be index 99")
	require.ErrorIs(t, sub.Err(), io.EOF, "subscriber should complete with io.EOF")
}

// TestRingbufSlowSubscriber validates MaxLag enforcement.
// A slow subscriber with MaxLag=10 gets ErrTooSlow while a
// normal subscriber keeps reading all events without error.
func TestRingbufSpikeSlowSubscriber(t *testing.T) {
	ctx := context.Background()
	rb := ringbuf.New[int](100) // minimum size is 100

	// Slow subscriber: MaxLag=10 → dropped when >10 items behind.
	slowSub := rb.Subscribe(ctx, &ringbuf.SubscribeOpts{
		Name:        "slow",
		StartBehind: 10,
		MaxLag:      10,
	})

	// Normal subscriber: MaxLag defaults to 50% of buffer (= 50).
	normalSub := rb.Subscribe(ctx, &ringbuf.SubscribeOpts{
		Name: "normal",
	})

	// Write enough events to make the slow subscriber fall behind.
	for i := range 50 {
		rb.Write(i)
	}
	rb.Close()

	// Normal subscriber drains all 50 events.
	var normalReceived []int
	for v := range normalSub.Iter() {
		normalReceived = append(normalReceived, v)
	}
	require.Lenf(t, normalReceived, 50, "normal subscriber should get all events")
	require.Truef(t, errors.Is(normalSub.Err(), io.EOF), "normal subscriber should end with io.EOF, got: %v", normalSub.Err())

	// Slow subscriber: at least 50 items behind, MaxLag=10 → ErrTooSlow.
	var slowReceived []int
	for v := range slowSub.Iter() {
		slowReceived = append(slowReceived, v)
	}
	require.Truef(t, errors.Is(slowSub.Err(), ringbuf.ErrTooSlow), "slow subscriber should get ErrTooSlow, got: %v", slowSub.Err())
	require.Lessf(t, len(slowReceived), 50, "slow subscriber should miss some events")
}
