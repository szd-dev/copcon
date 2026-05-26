import { describe, it, expect } from 'vitest';
import { applySSEChunk, createUserMessage, mergeMessages } from './message-reducer';
import type {
  CopConMessage,
  TextPart,
  ReasoningPart,
  ToolCallPart,
  InterruptPayload,
} from './types';

// Helper: build a minimal SSE JSON string
function sse(type: string, data: Record<string, unknown>): string {
  return JSON.stringify({ type, data });
}

// Helper: create a base assistant message for chaining
function baseMsg(overrides?: Partial<CopConMessage>): CopConMessage {
  return {
    id: 'test-msg',
    role: 'assistant',
    steps: [],
    metadata: { createdAt: '2025-01-01T00:00:00.000Z' },
    ...overrides,
  };
}

// Helper: apply multiple chunks in sequence
function applyChunks(msg: CopConMessage | null, chunks: string[]): CopConMessage {
  return chunks.reduce<CopConMessage>(
    (acc, chunk) => applySSEChunk(acc, chunk),
    msg ?? null,
  );
}

// ---------------------------------------------------------------------------
// applySSEChunk
// ---------------------------------------------------------------------------

describe('applySSEChunk', () => {
  // 1. step_create
  it('step_create: extends steps array, creates empty step at stepIndex', () => {
    const msg = baseMsg();
    const result = applySSEChunk(msg, sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    expect(result.steps).toHaveLength(1);
    expect(result.steps[0]).toEqual({ parts: [], status: 'streaming' });
  });

  // 2. part_create (text)
  it('part_create (text): inserts TextPart with state streaming', () => {
    const msg = applySSEChunk(baseMsg(), sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    const result = applySSEChunk(msg, sse('part_create', {
      messageId: 'test-msg',
      stepIndex: 0,
      partIndex: 0,
      partType: 'text',
    }));
    expect(result.steps[0].parts).toHaveLength(1);
    const part = result.steps[0].parts[0] as TextPart;
    expect(part.type).toBe('text');
    expect(part.text).toBe('');
    expect(part.state).toBe('streaming');
  });

  // 3. part_create (reasoning)
  it('part_create (reasoning): inserts ReasoningPart with state streaming', () => {
    const msg = applySSEChunk(baseMsg(), sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    const result = applySSEChunk(msg, sse('part_create', {
      messageId: 'test-msg',
      stepIndex: 0,
      partIndex: 0,
      partType: 'reasoning',
    }));
    expect(result.steps[0].parts).toHaveLength(1);
    const part = result.steps[0].parts[0] as ReasoningPart;
    expect(part.type).toBe('reasoning');
    expect(part.text).toBe('');
    expect(part.state).toBe('streaming');
  });

  // 4. part_create (tool-call)
  it('part_create (tool-call): inserts ToolCallPart with normalized state', () => {
    const msg = applySSEChunk(baseMsg(), sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    const result = applySSEChunk(msg, sse('part_create', {
      messageId: 'test-msg',
      stepIndex: 0,
      partIndex: 0,
      partType: 'tool-call',
      state: 'running',
      toolName: 'test_tool',
      toolCallId: 'call-123',
    }));
    expect(result.steps[0].parts).toHaveLength(1);
    const part = result.steps[0].parts[0] as ToolCallPart;
    expect(part.type).toBe('tool-call');
    expect(part.state).toBe('running');
    expect(part.toolName).toBe('test_tool');
    expect(part.output).toBe('');
    expect(part.error).toBe('');
  });

  // 5. part_update (textDelta) — incremental accumulation
  it('part_update (textDelta): accumulates text incrementally, not replaces', () => {
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'text' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, textDelta: 'Hello' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, textDelta: ' World' }),
    ]);
    const part = result.steps[0].parts[0] as TextPart;
    expect(part.text).toBe('Hello World');
  });

  // 6. part_update (state done)
  it('part_update (state done): updates part state to done', () => {
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'text' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, state: 'done' }),
    ]);
    const part = result.steps[0].parts[0] as TextPart;
    expect(part.state).toBe('done');
  });

  // 7. part_update (tool-call output)
  it('part_update (tool-call output): sets output field', () => {
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'tool-call', state: 'running', toolName: 'my_tool' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, output: '{"result": "ok"}' }),
    ]);
    const part = result.steps[0].parts[0] as ToolCallPart;
    expect(part.output).toBe('{"result": "ok"}');
  });

  // 8. part_update (tool-call error)
  it('part_update (tool-call error): sets error field', () => {
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'tool-call', state: 'running', toolName: 'my_tool' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, error: 'Something failed' }),
    ]);
    const part = result.steps[0].parts[0] as ToolCallPart;
    expect(part.error).toBe('Something failed');
  });

  // 9. part_update (tool-call interrupt)
  it('part_update (tool-call interrupt): sets interrupt payload and state waiting_for_input', () => {
    const interrupt: InterruptPayload = {
      interruptId: 'int-1',
      interruptType: 'approval',
      message: 'Please approve',
    };
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'tool-call', state: 'running', toolName: 'my_tool' }),
      sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, state: 'waiting_for_input', interrupt }),
    ]);
    const part = result.steps[0].parts[0] as ToolCallPart;
    expect(part.state).toBe('waiting_for_input');
    expect(part.interrupt).toEqual(interrupt);
  });

  // 10. message_done
  it('message_done: finalizes streaming→done, pending/running→complete', () => {
    const result = applyChunks(baseMsg(), [
      sse('step_create', { messageId: 'test-msg', stepIndex: 0 }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'text' }),
      sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 1, partType: 'tool-call', state: 'running', toolName: 'my_tool' }),
      sse('message_done', { messageId: 'test-msg' }),
    ]);
    expect(result.steps[0].status).toBe('done');
    const textPart = result.steps[0].parts[0] as TextPart;
    expect(textPart.state).toBe('done');
    const toolPart = result.steps[0].parts[1] as ToolCallPart;
    expect(toolPart.state).toBe('complete');
  });

  // 11. error event
  it('error: returns message unchanged', () => {
    const original = baseMsg({ steps: [{ parts: [], status: 'streaming' }] });
    const result = applySSEChunk(original, sse('error', { message: 'oops' }));
    expect(result).toBe(original);
  });

  // 12. filler-parts: partIndex > parts.length fills gaps with empty TextParts
  it('filler-parts: partIndex > parts.length fills gaps with empty TextParts', () => {
    const msg = applySSEChunk(baseMsg(), sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    const result = applySSEChunk(msg, sse('part_create', {
      messageId: 'test-msg',
      stepIndex: 0,
      partIndex: 2,
      partType: 'text',
    }));
    expect(result.steps[0].parts).toHaveLength(3);
    // Gap fillers at [0] and [1] are empty text parts
    const gap0 = result.steps[0].parts[0] as TextPart;
    const gap1 = result.steps[0].parts[1] as TextPart;
    expect(gap0.type).toBe('text');
    expect(gap0.text).toBe('');
    expect(gap1.type).toBe('text');
    expect(gap1.text).toBe('');
    // The actual created part at [2]
    const created = result.steps[0].parts[2] as TextPart;
    expect(created.type).toBe('text');
    expect(created.state).toBe('streaming');
  });

  // 13. step 0 implicit: part_create without prior step_create
  it('step 0 implicit: part_create with stepIndex=0 without prior step_create works', () => {
    const result = applySSEChunk(null, sse('part_create', {
      messageId: 'test-msg',
      stepIndex: 0,
      partIndex: 0,
      partType: 'text',
    }));
    expect(result.steps).toHaveLength(1);
    expect(result.steps[0].parts).toHaveLength(1);
    const part = result.steps[0].parts[0] as TextPart;
    expect(part.type).toBe('text');
    expect(part.state).toBe('streaming');
  });

  // 14. immutable: original message object is never mutated
  it('immutable: original message object is never mutated', () => {
    const original = baseMsg();
    const originalStepsLength = original.steps.length;
    const originalStepsRef = original.steps;
    applySSEChunk(original, sse('step_create', { messageId: 'test-msg', stepIndex: 0 }));
    applySSEChunk(original, sse('part_create', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, partType: 'text' }));
    applySSEChunk(original, sse('part_update', { messageId: 'test-msg', stepIndex: 0, partIndex: 0, textDelta: 'hi' }));
    expect(original.steps).toHaveLength(originalStepsLength);
    expect(original.steps).toBe(originalStepsRef);
  });

  // 15. events_lost: returns message unchanged
  it('events_lost: returns message unchanged', () => {
    const original = baseMsg({ steps: [{ parts: [], status: 'streaming' }] });
    const result = applySSEChunk(original, sse('events_lost', { message: 'connection lost' }));
    expect(result).toBe(original);
  });
});

// ---------------------------------------------------------------------------
// createUserMessage
// ---------------------------------------------------------------------------

describe('createUserMessage', () => {
  // 16. Creates correct structure with content
  it('creates correct structure with content', () => {
    const msg = createUserMessage('Hello');
    expect(msg.role).toBe('user');
    expect(msg.steps).toHaveLength(1);
    expect(msg.steps[0].status).toBe('done');
    expect(msg.steps[0].parts).toHaveLength(1);
    const part = msg.steps[0].parts[0] as TextPart;
    expect(part.type).toBe('text');
    expect(part.text).toBe('Hello');
    expect(part.state).toBe('done');
  });

  // 17. Empty content creates message with no steps
  it('empty content creates message with no steps', () => {
    const msg = createUserMessage('');
    expect(msg.role).toBe('user');
    expect(msg.steps).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// mergeMessages
// ---------------------------------------------------------------------------

describe('mergeMessages', () => {
  // 18. Deduplicates by id, fetched preferred
  it('deduplicates by id, fetched preferred over local', () => {
    const fetched: CopConMessage[] = [
      baseMsg({ id: 'msg1', role: 'assistant' }),
      baseMsg({ id: 'msg2', role: 'assistant' }),
    ];
    const local: CopConMessage[] = [
      baseMsg({ id: 'msg2', role: 'user' }), // duplicate id, different role
      baseMsg({ id: 'msg3', role: 'assistant' }),
    ];
    const result = mergeMessages(fetched, local);
    expect(result).toHaveLength(3);
    // msg2 should be the fetched version (assistant), not local (user)
    const msg2 = result.find((m) => m.id === 'msg2')!;
    expect(msg2.role).toBe('assistant');
  });

  // 19. Local-only messages preserved
  it('local-only messages are preserved', () => {
    const fetched: CopConMessage[] = [baseMsg({ id: 'msg1' })];
    const local: CopConMessage[] = [baseMsg({ id: 'msg2' })];
    const result = mergeMessages(fetched, local);
    expect(result).toHaveLength(2);
    expect(result.map((m) => m.id)).toEqual(['msg1', 'msg2']);
  });
});
