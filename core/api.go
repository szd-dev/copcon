package core

import (
	"github.com/copcon/core/agent"
	"github.com/copcon/core/chat"
	"github.com/copcon/core/storage"
)

type APIProvider interface {
	Store()        storage.StoreProvider
	Engine()       agent.AgentEngine
	Registry()     agent.AgentRegistry
	SessionStore() chat.SessionStore
}