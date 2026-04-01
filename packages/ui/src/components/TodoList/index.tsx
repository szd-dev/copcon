import React from 'react';
import { TodoItem, TodoItemProps } from '../TodoItem';

export interface TodoListProps {
  todos: TodoItemProps[];
  onStatusChange?: (id: string, status: string) => void;
  readonly?: boolean;
  emptyText?: string;
}

const statusPriority: Record<TodoItemProps['status'], number> = {
  in_progress: 0,
  pending: 1,
  blocked: 2,
  failed: 3,
  completed: 4,
};

export const TodoList: React.FC<TodoListProps> = ({
  todos,
  onStatusChange,
  readonly = false,
  emptyText = 'No todos yet',
}) => {
  const sortedTodos = [...todos].sort((a, b) => statusPriority[a.status] - statusPriority[b.status]);

  if (todos.length === 0) {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '48px 24px',
          color: '#8c8c8c',
          fontSize: '14px',
          textAlign: 'center',
        }}
      >
        <svg
          width="64"
          height="64"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          style={{ marginBottom: '16px', opacity: 0.3 }}
        >
          <path d="M9 11l3 3L22 4" />
          <path d="M21 12v7a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h11" />
        </svg>
        <span>{emptyText}</span>
      </div>
    );
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
      }}
    >
      {sortedTodos.map((todo) => (
        <TodoItem
          key={todo.id}
          {...todo}
          onStatusChange={onStatusChange}
          readonly={readonly}
        />
      ))}
    </div>
  );
};

export default TodoList;
