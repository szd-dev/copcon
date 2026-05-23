package storage

// StoreProvider aggregates all storage interfaces into a single provider.
// Implementations (e.g. core/providers/postgres.Store) supply all stores
// backed by the same database.
type StoreProvider interface {
	Sessions() SessionStore
	Messages() MessageStore
	Todos()    TodoStore
}