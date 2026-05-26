import { describe, it, expect } from 'vitest';
import { parseSSEStream, parseSSERaw } from './sse-parser';

function createMockReader(chunks: string[]): ReadableStreamDefaultReader<Uint8Array> {
  const encoder = new TextEncoder();
  let index = 0;
  return {
    read: async () => {
      if (index < chunks.length) {
        return { done: false, value: encoder.encode(chunks[index++]) };
      }
      return { done: true, value: undefined };
    },
  } as unknown as ReadableStreamDefaultReader<Uint8Array>;
}

describe('parseSSEStream', () => {
  it('parses a single event', async () => {
    const chunks: string[] = [];
    const reader = createMockReader([
      'data: {"type":"step_create","data":{}}\n\n',
    ]);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toBe('{"type":"step_create","data":{}}');
  });

  it('parses multiple consecutive events', async () => {
    const chunks: string[] = [];
    const reader = createMockReader([
      'data: event1\n\ndata: event2\n\n',
    ]);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(2);
    expect(chunks[0]).toBe('event1');
    expect(chunks[1]).toBe('event2');
  });

  it('handles multi-line data', async () => {
    const chunks: string[] = [];
    const reader = createMockReader([
      'data: line1\ndata: line2\n\n',
    ]);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toBe('line1\nline2');
  });

  it('handles trailing buffer without final blank line', async () => {
    const chunks: string[] = [];
    const reader = createMockReader(['data: trailing']);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toBe('trailing');
  });

  it('handles empty stream', async () => {
    const chunks: string[] = [];
    const reader = createMockReader(['']);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(0);
  });

  it('passes malformed data through without validation', async () => {
    const chunks: string[] = [];
    const reader = createMockReader(['data: not-json\n\n']);
    await parseSSEStream(reader, (raw) => chunks.push(raw));
    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toBe('not-json');
  });
});

describe('parseSSERaw', () => {
  it('parses valid JSON with type and data', () => {
    const result = parseSSERaw('{"type":"step_create","data":{"messageId":"abc"}}');
    expect(result).toEqual({ type: 'step_create', data: { messageId: 'abc' } });
  });

  it('returns undefined for non-JSON string', () => {
    expect(parseSSERaw('not json')).toBeUndefined();
  });

  it('returns undefined for JSON without type field', () => {
    expect(parseSSERaw('{"data":{}}')).toBeUndefined();
  });

  it('returns undefined for JSON without data field', () => {
    expect(parseSSERaw('{"type":"step_create"}')).toBeUndefined();
  });

  it('returns undefined when data is not an object', () => {
    expect(parseSSERaw('{"type":"step_create","data":"string"}')).toBeUndefined();
  });
});
