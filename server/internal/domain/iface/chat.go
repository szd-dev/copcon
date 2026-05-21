package iface

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/copcon/server/internal/domain/entity"
	"github.com/golang-cz/ringbuf"
	"github.com/google/uuid"
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
	RequestInput(req InputRequest) (*InputResponse, error)
	ResolveInput(interruptID string, resp *InputResponse) error
	PendingInputs() []InputRequest
	SetPartLocator(messageID string, stepIndex, partIndex int)
	ClearPartLocator()
}

// Storer removes a session from a store.
type Storer interface {
	Remove(sessionID string)
}

// Subscriber receives a filtered view of events emitted on a ChatContext.
type Subscriber struct {
	Events <-chan entity.Event
}

type partLocatorData struct {
	messageID string
	stepIndex int
	partIndex int
}

type ChatContext struct {
	ctx             context.Context
	sessionID       string
	agentID         string
	rb              *ringbuf.RingBuffer[entity.Event]
	seq             atomic.Int64
	depth           int
	closed          chan struct{}
	closeOnce       sync.Once
	store           Storer
	lifecycleCancel context.CancelFunc
	emitMu          sync.Mutex
	interruptMu     sync.Mutex
	interruptChans  map[string]chan *InputResponse
	interruptReqs   map[string]*InputRequest
	partLocator     *partLocatorData
}

type InterruptType string

const (
	InterruptApproval InterruptType = "approval"
	InterruptQuestion InterruptType = "question"
)

// InputRequest represents a request for human input during an agent execution.
type InputRequest struct {
	ID          string         `json:"id"`
	Type        InterruptType  `json:"type"`
	Message     string         `json:"message"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	ToolArgs    map[string]any `json:"tool_args,omitempty"`
}

// InputResponse represents a human's response to an InputRequest.
type InputResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

var ErrInterruptNotFound = errors.New("interrupt not found")

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
// This method is now concurrent-safe — it serializes access to the ring buffer
// via an internal mutex.
func (c *ChatContext) Emit(event entity.Event) {
	c.emitMu.Lock()
	c.rb.Write(event)
	c.seq.Add(1)
	c.emitMu.Unlock()
}

// Close terminates the lifecycle context, closes the ring buffer stream,
// signals completion via the closed channel, and removes the session from
// its associated store. Safe to call multiple times — only the first call
// has any effect.
func (c *ChatContext) Close() {
	c.closeOnce.Do(func() {
		c.lifecycleCancel()
		c.rb.Close()
		close(c.closed)
		if c.store != nil {
			c.store.Remove(c.sessionID)
		}
	})
}

// SetStore associates a store with this ChatContext.
// When Close() is called, the session is automatically removed from the store.
func (c *ChatContext) SetStore(s Storer) {
	c.store = s
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
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &ChatContext{
		ctx:             lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		sessionID:       sessionID,
		agentID:         agentID,
		rb:              ringbuf.New[entity.Event](1024),
		closed:          make(chan struct{}),
		interruptChans:  make(map[string]chan *InputResponse),
		interruptReqs:   make(map[string]*InputRequest),
	}
}

func (c *ChatContext) WithDepth(d int) *ChatContext {
	c.depth = d
	return c
}

// SetPartLocator stores part location info so RequestInput can emit correct part_update.
func (c *ChatContext) SetPartLocator(messageID string, stepIndex, partIndex int) {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.partLocator = &partLocatorData{messageID: messageID, stepIndex: stepIndex, partIndex: partIndex}
}

// ClearPartLocator removes part location info.
func (c *ChatContext) ClearPartLocator() {
	c.emitMu.Lock()
	defer c.emitMu.Unlock()
	c.partLocator = nil
}

func (c *ChatContext) RequestInput(req InputRequest) (*InputResponse, error) {
	id := uuid.New().String()
	req.ID = id

	ch := make(chan *InputResponse, 1)

	c.interruptMu.Lock()
	c.interruptChans[id] = ch
	c.interruptReqs[id] = &req
	c.interruptMu.Unlock()

	defer func() {
		c.interruptMu.Lock()
		delete(c.interruptChans, id)
		delete(c.interruptReqs, id)
		c.interruptMu.Unlock()
	}()

	c.emitMu.Lock()
	if c.partLocator != nil {
		event := entity.Event{
			Type: entity.EventPartUpdate,
			Data: entity.PartUpdateData{
				MessageID: c.partLocator.messageID,
				StepIndex: c.partLocator.stepIndex,
				PartIndex: c.partLocator.partIndex,
				PartType:  "tool-call",
				State:     string(entity.UIPartStateWaitingForInput),
				Interrupt: buildInterruptPayload(&req),
			},
		}
		c.rb.Write(event)
		c.seq.Add(1)
	}
	c.emitMu.Unlock()

	select {
	case resp := <-ch:
		return resp, nil
	case <-c.Closed():
		return nil, fmt.Errorf("session closed while waiting for input")
	}
}

func (c *ChatContext) ResolveInput(interruptID string, resp *InputResponse) error {
	c.interruptMu.Lock()
	ch, ok := c.interruptChans[interruptID]
	c.interruptMu.Unlock()
	if !ok {
		return ErrInterruptNotFound
	}
	ch <- resp
	return nil
}

func (c *ChatContext) PendingInputs() []InputRequest {
	c.interruptMu.Lock()
	defer c.interruptMu.Unlock()
	result := make([]InputRequest, 0, len(c.interruptReqs))
	for _, req := range c.interruptReqs {
		result = append(result, *req)
	}
	return result
}

func buildInterruptPayload(req *InputRequest) map[string]any {
	return map[string]any{
		"interruptId":   req.ID,
		"interruptType": string(req.Type),
		"message":       req.Message,
		"summary":       req.Summary,
		"inputSchema":   req.InputSchema,
	}
}

var _ ChatContextInterface = (*ChatContext)(nil)
