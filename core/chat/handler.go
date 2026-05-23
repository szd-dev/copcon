package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/copcon/core/agent"
	"github.com/copcon/core/chatcontext"
	"github.com/copcon/core/entity"
	"github.com/copcon/core/iface"
)

type ChatRequest struct {
	SessionID    string `json:"-"`
	Content      string `json:"content"`
	AgentID      string `json:"agent_id"`
	Reconnect    bool   `json:"reconnect"`
	LastEventSeq int64  `json:"last_event_seq"`
}

func HandleChat(
	ctx context.Context,
	w io.Writer,
	flusher http.Flusher,
	req ChatRequest,
	engine agent.AgentEngine,
	store SessionStore,
) error {
	if req.Reconnect {
		chatCtx, found := store.Get(req.SessionID)
		if !found {
			return nil
		}
		fromSeq := req.LastEventSeq + 1
		sub, found := chatCtx.Subscribe(fromSeq)
		if !found {
			data, _ := json.Marshal(entity.Event{Type: "events_lost"})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return nil
		}
		streamEvents(ctx, sub, w, flusher)
		return nil
	}

	if req.Content == "" {
		return fmt.Errorf("content is required")
	}
	if _, active := store.Get(req.SessionID); active {
		return fmt.Errorf("session already has an active agent")
	}

	chatCtx := chatcontext.NewChatContext(ctx, req.SessionID, req.AgentID)
	chatCtx.SetStore(store)
	store.Put(req.SessionID, chatCtx)

	go func() {
		defer func() {
			store.Remove(req.SessionID)
			chatCtx.Close()
		}()
		if err := engine.Chat(chatCtx, req.Content); err != nil {
			slog.Error("Agent chat error", "session_id", req.SessionID, "error", err)
		}
	}()

	sub, ok := chatCtx.Subscribe(0)
	if !ok {
		data, _ := json.Marshal(entity.Event{Type: "events_lost"})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return nil
	}
	streamEvents(ctx, sub, w, flusher)
	return nil
}

func streamEvents(ctx context.Context, sub *iface.Subscriber, w io.Writer, flusher http.Flusher) {
	for {
		select {
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}