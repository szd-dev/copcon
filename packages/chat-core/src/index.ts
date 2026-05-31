// @copcon/chat-core - Pure TypeScript chat core library

export { parseToolOutput } from './utils';
export type { ParsedToolOutput } from './utils';
export { parseSSEStream, parseSSERaw } from './sse-parser';
export { applySSEChunk, createUserMessage, mergeMessages } from './message-reducer';

export { AgentClient } from './agent-client';
export type { AgentClientConfig } from './agent-client';

export { ChatSession } from './chat-session';
export type { ChatSessionConfig } from './chat-session';

export { SubagentStream } from './subagent-stream';
export type { SubagentStreamConfig } from './subagent-stream';

export type {
  Session,
  Agent,
  ToolCall,
  Todo,
  AsyncToolStartedData,
  AsyncToolCompleteData,
  AsyncToolFailedData,
  AsyncCompletionPendingData,
  InterruptPayload,
  ToolExecution,
  TextPart,
  ReasoningPart,
  ToolCallPart,
  Part,
  Step,
  UIMessageMeta,
  CopConMessage,
  CopConInput,
  CopConSSEOutput,
  SSEEventType,
  StepCreateEvent,
  PartCreateEvent,
  PartUpdateEvent,
  MessageDoneEvent,
  SessionStatus,
  SessionState,
  ChatSessionCallbacks,
  KnowledgeBase,
  DocumentStatus,
  Document,
  Chunk,
  SearchResult,
  MemoryType,
  Memory,
} from './types';