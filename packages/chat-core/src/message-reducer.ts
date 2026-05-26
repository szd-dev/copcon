import type {
  CopConMessage,
  Step,
  Part,
  TextPart,
  ReasoningPart,
  ToolCallPart,
  InterruptPayload,
} from './types';

// --- Internal helpers ---

function normalizeToolCallState(
  raw: unknown,
  fallback: ToolCallPart['state'] = 'pending',
): ToolCallPart['state'] {
  if (raw === 'pending') return 'pending';
  if (raw === 'running') return 'running';
  if (raw === 'complete') return 'complete';
  if (raw === 'error') return 'error';
  if (raw === 'waiting_for_input') return 'waiting_for_input';
  return fallback;
}

function ensureSteps(steps: Step[], stepIndex: number): Step[] {
  const result = [...steps];
  while (result.length <= stepIndex) {
    result.push({ parts: [], status: 'streaming' });
  }
  return result;
}

function ensureParts(parts: Part[], partIndex: number): Part[] {
  const result = [...parts];
  while (result.length <= partIndex) {
    result.push({ type: 'text', text: '', state: 'streaming' });
  }
  return result;
}

function createEmptyMessage(): CopConMessage {
  return {
    id: `assistant-${Date.now()}`,
    role: 'assistant',
    steps: [],
    metadata: { createdAt: new Date().toISOString() },
  };
}

// --- Internal event handlers ---

function handleStepCreate(
  baseMessage: CopConMessage,
  data: Record<string, unknown>,
): CopConMessage {
  const stepIndex =
    typeof data.stepIndex === 'number' ? data.stepIndex : 0;

  const steps = ensureSteps([...baseMessage.steps], stepIndex);
  steps[stepIndex] = { parts: [], status: 'streaming' };

  return { ...baseMessage, steps };
}

function handlePartCreate(
  baseMessage: CopConMessage,
  data: Record<string, unknown>,
): CopConMessage {
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
      const state = normalizeToolCallState(data.state, 'pending');
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
        ...(data.interrupt ? { interrupt: data.interrupt as InterruptPayload } : {}),
      };
      break;
    }
    default:
      return baseMessage;
  }

  const steps = ensureSteps([...baseMessage.steps], stepIndex);
  const parts = ensureParts([...steps[stepIndex].parts], partIndex);
  parts[partIndex] = newPart;
  steps[stepIndex] = { ...steps[stepIndex], parts };

  return { ...baseMessage, steps };
}

function handlePartUpdate(
  baseMessage: CopConMessage,
  data: Record<string, unknown>,
): CopConMessage {
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
      const state = normalizeToolCallState(data.state, part.state);
      updatedPart = {
        ...part,
        ...(state !== part.state ? { state } : {}),
        ...(data.output !== undefined && typeof data.output === 'string'
          ? { output: data.output }
          : {}),
        ...(data.error !== undefined && typeof data.error === 'string'
          ? { error: data.error }
          : {}),
        ...(data.interrupt !== undefined ? { interrupt: data.interrupt as InterruptPayload } : {}),
      };
      break;
    }
  }

  parts[partIndex] = updatedPart;
  steps[stepIndex] = { ...step, parts };

  return { ...baseMessage, steps };
}

function handleMessageDone(baseMessage: CopConMessage): CopConMessage {
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

// --- Exported functions ---

export function applySSEChunk(originMessage: CopConMessage | null, rawData: string): CopConMessage {
  let parsedData: { type?: string; data?: Record<string, unknown> } | undefined;
  try {
    if (rawData) {
      parsedData = JSON.parse(rawData);
    }
  } catch {
    return originMessage ?? createEmptyMessage();
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

  if (!parsedData) return baseMessage;

  const { type, data } = parsedData;
  if (!data) return baseMessage;

  switch (type) {
    case 'step_create': return handleStepCreate(baseMessage, data);
    case 'part_create': return handlePartCreate(baseMessage, data);
    case 'part_update': return handlePartUpdate(baseMessage, data);
    case 'message_done': return handleMessageDone(baseMessage);
    case 'error': return baseMessage;
    case 'events_lost': return baseMessage;
    case 'async_tool_started': // TODO: handle async_tool events
    case 'async_tool_complete':
    case 'async_tool_failed': return baseMessage;
    default: return baseMessage;
  }
}

export function createUserMessage(content: string): CopConMessage {
  return {
    id: `local-${Date.now()}`,
    role: 'user',
    steps: content
      ? [{ parts: [{ type: 'text', text: content, state: 'done' }], status: 'done' }]
      : [],
    metadata: { createdAt: new Date().toISOString() },
  };
}

export function mergeMessages(
  fetched: CopConMessage[],
  local: CopConMessage[],
): CopConMessage[] {
  const fetchedIdSet = new Set(fetched.map((m) => m.id));
  const merged = [...fetched];
  for (const msg of local) {
    if (!fetchedIdSet.has(msg.id)) {
      merged.push(msg);
    }
  }
  return merged;
}
