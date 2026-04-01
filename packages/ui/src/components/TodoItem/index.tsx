import React from 'react';
import {
  MinusCircleOutlined,
  LoadingOutlined,
  CheckCircleOutlined,
  LockOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons';

export interface TodoItemProps {
  id: string;
  content: string;
  status: 'pending' | 'in_progress' | 'completed' | 'blocked' | 'failed';
  activeForm?: string;
  result?: string;
  onStatusChange?: (id: string, status: string) => void;
  readonly?: boolean;
}

const statusIcons: Record<TodoItemProps['status'], React.ReactNode> = {
  pending: <MinusCircleOutlined />,
  in_progress: <LoadingOutlined spin />,
  completed: <CheckCircleOutlined style={{ color: '#52c41a' }} />,
  blocked: <LockOutlined />,
  failed: <CloseCircleOutlined style={{ color: '#ff4d4f' }} />,
};

const statusLabels: Record<TodoItemProps['status'], string> = {
  pending: '待处理',
  in_progress: '进行中',
  completed: '已完成',
  blocked: '已阻塞',
  failed: '已失败',
};

export const TodoItem: React.FC<TodoItemProps> = ({
  id,
  content,
  status,
  activeForm,
  result,
  onStatusChange,
  readonly = false,
}) => {
  const handleStatusClick = () => {
    if (readonly || !onStatusChange) return;

    const statusOrder: TodoItemProps['status'][] = [
      'pending',
      'in_progress',
      'completed',
    ];
    const currentIndex = statusOrder.indexOf(status);
    const nextStatus = statusOrder[(currentIndex + 1) % statusOrder.length];
    onStatusChange(id, nextStatus);
  };

  return (
    <div className="todo-item" data-status={status} data-id={id}>
      <div className="todo-item-header">
        <span
          className="todo-item-status-icon"
          onClick={handleStatusClick}
          role={readonly ? undefined : 'button'}
          tabIndex={readonly ? undefined : 0}
          aria-label={`状态: ${statusLabels[status]}`}
        >
          {statusIcons[status]}
        </span>
        <span className="todo-item-content">{content}</span>
      </div>

      {status === 'in_progress' && activeForm && (
        <div className="todo-item-active-form">
          <div className="todo-item-section-title">执行中表单</div>
          <div className="todo-item-form-content">{activeForm}</div>
        </div>
      )}

      {status === 'completed' && result && (
        <div className="todo-item-result">
          <div className="todo-item-section-title">执行结果</div>
          <div className="todo-item-result-content">{result}</div>
        </div>
      )}
    </div>
  );
};

export default TodoItem;
