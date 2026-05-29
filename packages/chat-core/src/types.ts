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

// --- Async Tool Data Types ---

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

export interface InterruptPayload {
  interruptId: string;
  interruptType: 'approval' | 'question';
  message: string;
  summary?: string;
  inputSchema?: Record<string, unknown>;
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

// --- UI Message Types (Step-based) ---

export interface TextPart {
  type: 'text';
  text: string;
  state: 'streaming' | 'done';
}

export interface ReasoningPart {
  type: 'reasoning';
  text: string;
  state: 'streaming' | 'done';
}

export interface ToolCallPart {
  type: 'tool-call';
  toolCallId: string;
  toolName: string;
  args: string;
  output: string;
  error: string;
  state: 'pending' | 'running' | 'complete' | 'error' | 'waiting_for_input';
  interrupt?: InterruptPayload;
}

export type Part = TextPart | ReasoningPart | ToolCallPart;

export interface Step {
  parts: Part[];
  status: 'streaming' | 'done';
}

export interface UIMessageMeta {
  createdAt: string;
  model?: string;
  tokenCount?: number;
  durationMs?: number;
}

// --- CopCon Message (primary message type, merges UIMessage) ---

export interface CopConMessage {
  id: string;
  role: 'user' | 'assistant';
  steps: Step[];
  metadata: UIMessageMeta;
}

export interface CopConInput {
  content: string;
  agentId?: string;
  sessionId: string;
}

export interface CopConSSEOutput {
  data: string;
}

// --- SSE Event Types ---

export type SSEEventType =
  | 'step_create'
  | 'part_create'
  | 'part_update'
  | 'message_done'
  | 'error'
  | 'async_tool_started'
  | 'async_tool_complete'
  | 'async_tool_failed';

export interface StepCreateEvent {
  type: 'step_create';
  data: { messageId: string; stepIndex: number };
}

export interface PartCreateEvent {
  type: 'part_create';
  data: {
    messageId: string;
    stepIndex: number;
    partIndex: number;
    partType: string;
    state?: string;
    toolCallId?: string;
    toolName?: string;
    args?: string;
    interrupt?: InterruptPayload;
  };
}

export interface PartUpdateEvent {
  type: 'part_update';
  data: {
    messageId: string;
    stepIndex: number;
    partIndex: number;
    partType: string;
    textDelta?: string;
    state?: string;
    output?: string;
    error?: string;
    interrupt?: InterruptPayload;
  };
}

export interface MessageDoneEvent {
  type: 'message_done';
  data: { messageId: string };
}

// --- Session State ---

export type SessionStatus = 'idle' | 'streaming' | 'reconnecting' | 'error';

export interface SessionState {
  status: SessionStatus;
  error: Error | undefined;
}

export interface ChatSessionCallbacks {
  onMessagesChange: (messages: CopConMessage[]) => void;
  onStateChange: (state: SessionState) => void;
}

export interface KnowledgeBase {
  id: string;
  name: string;
  backend: string;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  metadata: Record<string, unknown>;
}

export type DocumentStatus = 'pending' | 'parsing' | 'ready' | 'error';

export interface Document {
  id: string;
  kb_id: string;
  filename: string;
  source: string;
  status: DocumentStatus;
  chunk_count: number;
  token_count: number;
  created_at: string;
  updated_at: string;
  metadata: Record<string, unknown>;
}

export interface Chunk {
  id: string;
  document_id: string;
  kb_id: string;
  content: string;
  context: string;
  index: number;
  token_count: number;
  metadata: Record<string, unknown>;
  score: number;
}

export interface SearchResult {
  results: Chunk[];
}

export type MemoryType = 'episodic' | 'semantic' | 'procedural';

export interface Memory {
  id: string;
  content: string;
  session_id: string;
  role: string;
  timestamp: string;
  memory_type: string;
  metadata: Record<string, unknown>;
  score: number;
  importance: number;
}