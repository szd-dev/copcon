import { AbstractChatProvider, TransformMessage } from '@ant-design/x-sdk';
import type { XRequestOptions } from '@ant-design/x-sdk';
import type {
  Step,
  Part,
  TextPart,
  ReasoningPart,
  ToolCallPart,
  UIMessageMeta,
} from '../api/types';

/**
 * CopCon message structure using step-based architecture.
 * Each message contains steps, and each step contains parts
 * (text, reasoning, tool-call).
 */
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

/**
 * SSE output structure from backend.
 * Backend sends: data: {"type": "step_create", "data": {...}}
 * XStream parses this as: { data: '{"type": "step_create", "data": {...}}' } (STRING!)
 * The data field is a JSON string that must be parsed.
 */
export interface CopConSSEOutput {
  data: string;
}

/**
 * CopCon Chat Provider - Adapts backend SSE API to @ant-design/x-sdk
 *
 * Handles step-based events only:
 * - step_create: creates a new step with empty parts
 * - part_create: creates a new part within a step
 * - part_update: updates a part's content/state
 * - message_done: finalizes all steps and parts
 * - error: logs error from backend
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
    const text = requestParams.content || '';

    return {
      id: `local-${Date.now()}`,
      role: 'user',
      steps: text
        ? [{ parts: [{ type: 'text', text, state: 'done' }], status: 'done' }]
        : [],
      metadata: { createdAt: new Date().toISOString() },
    };
  }

  transformMessage(info: TransformMessage<CopConMessage, CopConSSEOutput>): CopConMessage {
    const { originMessage, chunk } = info;

    // Parse SSE data - XStream returns chunk.data as a JSON STRING, not an object
    let parsedData: { type?: string; data?: Record<string, unknown> } | undefined;
    if (chunk?.data) {
      parsedData = typeof chunk.data === 'string' ? JSON.parse(chunk.data) : chunk.data;
    }

    const chunkMessageId =
      typeof parsedData?.data?.messageId === 'string'
        ? parsedData.data.messageId
        : undefined;

    const baseMessage: CopConMessage = originMessage ?? {
      id: chunkMessageId ?? `assistant-${Date.now()}`,
      role: 'assistant',
      steps: [],
      metadata: { createdAt: new Date().toISOString() },
    };

    if (!parsedData) {
      return baseMessage;
    }

    const { type, data } = parsedData;

    if (!data) {
      return baseMessage;
    }

    switch (type) {
      // ──────────────────────────────────────────────────────────
      // step_create: creates a new step at the specified index
      // ──────────────────────────────────────────────────────────
      case 'step_create': {
        const stepIndex =
          typeof data.stepIndex === 'number' ? data.stepIndex : 0;

        const steps = [...baseMessage.steps];
        while (steps.length <= stepIndex) {
          steps.push({ parts: [], status: 'streaming' });
        }
        steps[stepIndex] = { parts: [], status: 'streaming' };

        return { ...baseMessage, steps };
      }

      // ──────────────────────────────────────────────────────────
      // part_create: creates a new part within a step at the specified index
      // ──────────────────────────────────────────────────────────
      case 'part_create': {
        const stepIndex =
          typeof data.stepIndex === 'number' ? data.stepIndex : 0;
        const partIndex =
          typeof data.partIndex === 'number' ? data.partIndex : 0;
        const partType =
          typeof data.partType === 'string' ? data.partType : 'text';

        let newPart: Part;
        switch (partType) {
          case 'text':
            newPart = { type: 'text', text: '', state: 'streaming' };
            break;
          case 'reasoning':
            newPart = { type: 'reasoning', text: '', state: 'streaming' };
            break;
          case 'tool-call': {
            const state: ToolCallPart['state'] =
              data.state === 'pending' ? 'pending' :
              data.state === 'running' ? 'running' :
              data.state === 'complete' ? 'complete' :
              data.state === 'error' ? 'error' :
              'pending';
            newPart = {
              type: 'tool-call',
              toolCallId:
                typeof data.toolCallId === 'string' ? data.toolCallId : '',
              toolName:
                typeof data.toolName === 'string' ? data.toolName : '',
              args: typeof data.args === 'string' ? data.args : '{}',
              output: '',
              error: '',
              state,
            };
            break;
          }
          default:
            return baseMessage;
        }

        // Ensure steps array is large enough (immutable — deep copy)
        const steps = [...baseMessage.steps];
        while (steps.length <= stepIndex) {
          steps.push({ parts: [], status: 'streaming' });
        }

        // Insert newPart at partIndex within the target step (immutable)
        const parts = [...steps[stepIndex].parts];
        while (parts.length <= partIndex) {
          parts.push({ type: 'text', text: '', state: 'streaming' });
        }
        parts[partIndex] = newPart;

        steps[stepIndex] = { ...steps[stepIndex], parts };

        return { ...baseMessage, steps };
      }

      // ──────────────────────────────────────────────────────────
      // part_update: updates a part's content/state at the specified index
      // ──────────────────────────────────────────────────────────
      case 'part_update': {
        const stepIndex =
          typeof data.stepIndex === 'number' ? data.stepIndex : 0;
        const partIndex =
          typeof data.partIndex === 'number' ? data.partIndex : 0;

        if (stepIndex >= baseMessage.steps.length) {
          return baseMessage;
        }

        const steps = [...baseMessage.steps];
        const step = steps[stepIndex];

        if (partIndex >= step.parts.length) {
          return baseMessage;
        }

        const parts = [...step.parts];
        const part = parts[partIndex];

        let updatedPart: Part;
        switch (part.type) {
          case 'text': {
            const textDelta =
              typeof data.textDelta === 'string' ? data.textDelta : '';
            const state: TextPart['state'] =
              data.state === 'done' ? 'done' : part.state;
            updatedPart = { ...part, text: part.text + textDelta, state };
            break;
          }
          case 'reasoning': {
            const textDelta =
              typeof data.textDelta === 'string' ? data.textDelta : '';
            const state: ReasoningPart['state'] =
              data.state === 'done' ? 'done' : part.state;
            updatedPart = { ...part, text: part.text + textDelta, state };
            break;
          }
          case 'tool-call': {
            const state: ToolCallPart['state'] =
              data.state === 'pending' ? 'pending' :
              data.state === 'running' ? 'running' :
              data.state === 'complete' ? 'complete' :
              data.state === 'error' ? 'error' :
              part.state;
            updatedPart = {
              ...part,
              ...(state !== part.state ? { state } : {}),
              ...(data.output !== undefined && typeof data.output === 'string'
                ? { output: data.output }
                : {}),
              ...(data.error !== undefined && typeof data.error === 'string'
                ? { error: data.error }
                : {}),
            };
            break;
          }
        }

        parts[partIndex] = updatedPart;
        steps[stepIndex] = { ...step, parts };

        return { ...baseMessage, steps };
      }

      // ──────────────────────────────────────────────────────────
      // message_done: finalizes all steps and parts
      // ──────────────────────────────────────────────────────────
      case 'message_done': {
        const steps = baseMessage.steps.map((step) => ({
          ...step,
          status: 'done' as const,
          parts: step.parts.map((part) => {
            if (part.type === 'text' && part.state === 'streaming') {
              return { ...part, state: 'done' as const };
            }
            if (part.type === 'reasoning' && part.state === 'streaming') {
              return { ...part, state: 'done' as const };
            }
            if (
              part.type === 'tool-call' &&
              (part.state === 'pending' || part.state === 'running')
            ) {
              return { ...part, state: 'complete' as const };
            }
            return part;
          }),
        }));

        return { ...baseMessage, steps };
      }

      // ──────────────────────────────────────────────────────────
      // error: log and return message unchanged
      // ──────────────────────────────────────────────────────────
      case 'error': {
        console.error('[CopConChatProvider] Error from backend:', data.error);
        return baseMessage;
      }

      default:
        return baseMessage;
    }
  }
}
