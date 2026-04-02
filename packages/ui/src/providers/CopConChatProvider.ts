import { AbstractChatProvider, TransformMessage } from '@ant-design/x-sdk';
import type { XRequestOptions } from '@ant-design/x-sdk';
import { parseToolOutput } from '../utils/messageUtils';

/**
 * CopCon message structure matching backend SSE format
 */
export interface CopConMessage {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  reasoning?: string;
  tool_calls?: Array<{
    id: string;
    type: 'function';
    function: {
      name: string;
      arguments: string;
    };
    status?: 'loading' | 'success' | 'error' | 'abort';
    output?: string;
  }>;
  tool_call_id?: string;
  created_at: string;
}

/**
 * Input parameters for chat request
 */
export interface CopConInput {
  content: string;
  agentId?: string;
  sessionId: string;
}

/**
 * SSE output structure from backend
 * Backend sends: data: {"type": "message", "data": {...}}
 * XRequest parses this as: { data: {"type": "message", "data": {...}} }
 */
export interface CopConSSEOutput {
  data: {
    type: 'message' | 'reasoning' | 'tool_call' | 'tool_result' | 'done' | 'error';
    data: Record<string, unknown>;
  };
}

/**
 * CopCon Chat Provider - Adapts backend SSE API to @ant-design/x-sdk
 *
 * Usage:
 * ```tsx
 * import { XRequest } from '@ant-design/x-sdk';
 * import { CopConChatProvider } from '@copcon/ui';
 *
 * const request = XRequest('http://localhost:8080/api/sessions/session-id/chat', {
 *   params: { content: '' },
 * });
 *
 * const provider = new CopConChatProvider({ request });
 * ```
 */
export default class CopConChatProvider extends AbstractChatProvider<
  CopConMessage,
  CopConInput,
  CopConSSEOutput
> {
  transformParams(
    requestParams: Partial<CopConInput>,
    options: XRequestOptions<CopConInput, CopConSSEOutput, CopConMessage>
  ): CopConInput {
    const baseParams = options.params || {};

    return {
      content: requestParams.content || baseParams.content || '',
      agentId: requestParams.agentId || baseParams.agentId,
      sessionId: requestParams.sessionId || baseParams.sessionId || '',
    };
  }

  transformLocalMessage(requestParams: Partial<CopConInput>): CopConMessage {
    const now = new Date().toISOString();

    return {
      id: `local-${Date.now()}`,
      session_id: requestParams.sessionId || '',
      role: 'user',
      content: requestParams.content || '',
      created_at: now,
    };
  }

  transformMessage(info: TransformMessage<CopConMessage, CopConSSEOutput>): CopConMessage {
    const { originMessage, chunk } = info;

    // Extract message_id from chunk for message grouping
    // Backend sends message_id with each message event - use it to group chunks
    const chunkMessageId = chunk?.data?.data?.message_id as string | undefined;

    const baseMessage: CopConMessage = originMessage || {
      // Use message_id from backend if available, otherwise generate temp ID
      id: chunkMessageId || `assistant-${Date.now()}`,
      session_id: '',
      role: 'assistant',
      content: '',
      created_at: new Date().toISOString(),
    };

    if (!chunk?.data) {
      return baseMessage;
    }

    const { type, data } = chunk.data;

    switch (type) {
      case 'message':
        return {
          ...baseMessage,
          content: baseMessage.content + (data.content as string || ''),
        };

      case 'reasoning':
        return {
          ...baseMessage,
          reasoning: (baseMessage.reasoning || '') + (data.content as string || ''),
        };

      case 'tool_call': {
        const funcData = data.function as { name?: string; arguments?: string } | undefined;
        const toolCall = {
          id: data.id as string || `tool-${Date.now()}`,
          type: 'function' as const,
          function: {
            name: (data.name as string) || funcData?.name || '',
            arguments: (data.arguments as string) || funcData?.arguments || '{}',
          },
        };
        return {
          ...baseMessage,
          tool_calls: [...(baseMessage.tool_calls || []), toolCall],
        };
      }

      case 'tool_result': {
        const toolCallId = data.id as string;
        const toolCalls = baseMessage.tool_calls;

        if (!toolCalls) {
          console.warn('[CopConChatProvider] tool_result received but no tool_calls in message');
          return baseMessage;
        }

        const toolCallIndex = toolCalls.findIndex(tc => tc.id === toolCallId);
        if (toolCallIndex === -1) {
          console.warn(`[CopConChatProvider] tool_result received for unknown tool_call_id: ${toolCallId}`);
          return baseMessage;
        }

        const rawOutput = (data.output as string) || (data.result as string) || (data.content as string) || '';
        const { status, output } = parseToolOutput(rawOutput);

        const updatedToolCalls = [...toolCalls];
        updatedToolCalls[toolCallIndex] = {
          ...toolCalls[toolCallIndex],
          status,
          output,
        };

        return {
          ...baseMessage,
          tool_calls: updatedToolCalls,
        };
      }

      case 'done':
        return baseMessage;

      case 'error':
        console.error('[CopConChatProvider] Error from backend:', data.error);
        return baseMessage;

      default:
        return baseMessage;
    }
  }
}
