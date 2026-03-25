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

export type SSEEventType = 'message' | 'reasoning' | 'tool_call' | 'tool_result' | 'thought' | 'done' | 'error';

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
  };
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