import type { Todo } from '@copcon/chat-core';

interface TodoListProps {
  todos: Todo[];
  onStatusChange?: (id: string, status: string) => void;
  readonly?: boolean;
}

const STATUS_PRIORITY: Record<string, number> = {
  in_progress: 0,
  pending: 1,
  blocked: 2,
  failed: 3,
  completed: 4,
};

const STATUS_ICONS: Record<string, string> = {
  pending: '○',
  in_progress: '◐',
  completed: '●',
  blocked: '⊘',
  failed: '✕',
};

const STATUS_COLORS: Record<string, string> = {
  pending: '#999',
  in_progress: '#1677ff',
  completed: '#52c41a',
  blocked: '#faad14',
  failed: '#ff4d4f',
};

const STATUS_CYCLE = ['pending', 'in_progress', 'completed'] as const;

export function TodoList({ todos, onStatusChange, readonly }: TodoListProps) {
  const sorted = [...todos].sort(
    (a, b) => (STATUS_PRIORITY[a.status] ?? 99) - (STATUS_PRIORITY[b.status] ?? 99)
  );

  if (sorted.length === 0) {
    return (
      <div style={{ textAlign: 'center', padding: 24, color: '#999' }}>
        <div style={{ fontSize: 24 }}>✓</div>
        <div>No todos yet</div>
      </div>
    );
  }

  return (
    <div>
      {sorted.map((todo) => (
        <div key={todo.id} style={{ padding: '6px 0', borderBottom: '1px solid #f0f0f0' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span
              onClick={() => {
                if (readonly || !onStatusChange) return;
                const currentIdx = STATUS_CYCLE.indexOf(todo.status as typeof STATUS_CYCLE[number]);
                if (currentIdx >= 0) {
                  onStatusChange(todo.id, STATUS_CYCLE[(currentIdx + 1) % STATUS_CYCLE.length]);
                }
              }}
              style={{
                cursor: readonly ? 'default' : 'pointer',
                color: STATUS_COLORS[todo.status] || '#999',
                fontSize: 16,
                animation: todo.status === 'in_progress' ? 'spin 1s linear infinite' : undefined,
              }}
            >
              {STATUS_ICONS[todo.status] || '○'}
            </span>
            <span style={{ flex: 1, fontSize: 13 }}>
              {todo.active_form && todo.status === 'in_progress' ? todo.active_form : todo.content}
            </span>
          </div>
          {todo.result && todo.status === 'completed' && (
            <div style={{ paddingLeft: 24, fontSize: 12, color: '#666', marginTop: 2 }}>
              {todo.result}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}