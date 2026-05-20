package iface

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/golang-cz/ringbuf"
)

type ChatContextInterface interface {
	Context() context.Context
	SessionID() string
	AgentID() string
	Events() <-chan entity.Event
	Emit(event entity.Event)
	Close()
	Closed() <-chan struct{}
	Depth() int
	Subscribe(fromSeq int64) (*Subscriber, bool)
}

// Subscriber receives a filtered view of events emitted on a ChatContext.
type Subscriber struct {
	Events <-chan entity.Event
}

type ChatContext struct {
	ctx       context.Context
	sessionID string
	agentID   string
	rb        *ringbuf.RingBuffer[entity.Event]
	seq       atomic.Int64
	depth     int
	closed    chan struct{}
}

func (c *ChatContext) Context() context.Context { return c.ctx }
func (c *ChatContext) SessionID() string        { return c.sessionID }
func (c *ChatContext) AgentID() string          { return c.agentID }

// Events returns a backward-compatible <-chan entity.Event.
// It creates a ringbuf subscriber that tails the entire buffer and
// forwards events via a goroutine. The channel closes when the
// ring buffer is closed (io.EOF) or the subscriber errors.
func (c *ChatContext) Events() <-chan entity.Event {
	sub := c.rb.Subscribe(context.Background(), &ringbuf.SubscribeOpts{
		Name:        "events",
		StartBehind: c.rb.Size(), // capture full buffer window
		MaxLag:      c.rb.Size(),
	})

	ch := make(chan entity.Event, 256)
	go func() {
		defer close(ch)
		for event := range sub.Iter() {
			ch <- event
		}
	}()

	return ch
}

// Emit writes an event to the ring buffer and increments the sequence counter.
// The ringbuf Write is NOT concurrent-safe; the caller must serialize Emit calls.
// This matches the original single-writer pattern from chan-based Emit.
func (c *ChatContext) Emit(event entity.Event) {
	c.rb.Write(event)
	c.seq.Add(1)
}

// Close terminates the ring buffer stream and signals completion via the closed channel.
// After Close(), subscribers drain remaining data then receive io.EOF.
func (c *ChatContext) Close() {
	c.rb.Close()
	close(c.closed)
}

// Closed returns a channel that fires once after Close() completes.
func (c *ChatContext) Closed() <-chan struct{} {
	return c.closed
}

func (c *ChatContext) Depth() int {
	return c.depth
}

// Subscribe creates a ringbuf subscriber starting from the given sequence number.
// Returns (nil, false) if fromSeq has been evicted from the buffer window or is invalid.
// If successful, the returned Subscriber delivers all events from fromSeq onward
// until the ring buffer is closed.
func (c *ChatContext) Subscribe(fromSeq int64) (*Subscriber, bool) {
	if fromSeq < 0 {
		return nil, false
	}

	currentSeq := c.seq.Load()
	lag := currentSeq - fromSeq

	// fromSeq is in the future or has been evicted from the buffer.
	if lag < 0 || lag > int64(c.rb.Size()) {
		return nil, false
	}

	startBehind := uint64(lag)

	sub := c.rb.Subscribe(context.Background(), &ringbuf.SubscribeOpts{
		Name:        fmt.Sprintf("sub-%d", fromSeq),
		StartBehind: startBehind,
		MaxLag:      c.rb.Size(),
	})

	ch := make(chan entity.Event, 256)
	go func() {
		defer close(ch)
		for event := range sub.Iter() {
			ch <- event
		}
	}()

	return &Subscriber{Events: ch}, true
}

func NewChatContext(ctx context.Context, sessionID, agentID string) *ChatContext {
	return &ChatContext{
		ctx:       ctx,
		sessionID: sessionID,
		agentID:   agentID,
		rb:        ringbuf.New[entity.Event](1024),
		closed:    make(chan struct{}),
	}
}

func (c *ChatContext) WithDepth(d int) *ChatContext {
	c.depth = d
	return c
}

var _ ChatContextInterface = (*ChatContext)(nil)
