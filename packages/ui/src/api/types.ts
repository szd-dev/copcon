export interface Session {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

export interface Message {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  reasoning?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  created_at: string;
}

export interface Todo {
  id: string;
  session_id: string;
  content: string;
  status: 'pending' | 'in_progress' | 'completed' | 'blocked' | 'failed';
  active_form?: string;
  result?: string;
  depends_on?: string[];
  retry_count: number;
  created_at: string;
  updated_at: string;
}

export type SSEEventType = 'message' | 'reasoning' | 'tool_call' | 'tool_result' | 'thought' | 'done' | 'error' | 'async_tool_started' | 'async_tool_complete' | 'async_tool_failed' | 'async_completion_pending';

export interface SSEEvent {
  type: SSEEventType;
  data: {
    content?: string;
    tool_name?: string;
    tool_args?: Record<string, unknown>;
    result?: unknown;
    message_id?: string;
    error?: string;
    id?: string;
    name?: string;
    arguments?: string;
    output?: string;
    call_id?: string;
    session_id?: string;
    duration_ms?: number;
    completed_at?: string;
    status?: string;
  };
}

export interface AsyncToolStartedData {
  call_id: string;
  tool_name: string;
  session_id: string;
}

export interface AsyncToolCompleteData {
  call_id: string;
  tool_name: string;
  result: unknown;
  duration_ms: number;
}

export interface AsyncToolFailedData {
  call_id: string;
  tool_name: string;
  error: string;
  duration_ms: number;
}

export interface AsyncCompletionPendingData {
  call_id: string;
  tool_name: string;
  session_id: string;
  completed_at: string;
}

export interface ToolExecution {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
  output?: string;
  status: 'running' | 'success' | 'error';
  startTime: number;
  endTime?: number;
}