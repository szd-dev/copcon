/**
 * Parse an SSE stream from a ReadableStream reader.
 * Calls onChunk for each complete SSE event (data: lines separated by blank lines).
 */
export async function parseSSEStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  onChunk: (rawData: string) => void,
): Promise<void> {
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    let currentData = '';

    for (const line of lines) {
      if (line.startsWith('data: ')) {
        currentData += (currentData ? '\n' : '') + line.slice(6);
      } else if (line === '' && currentData) {
        onChunk(currentData);
        currentData = '';
      }
    }
  }

  const remainder = buffer.trim();
  if (remainder.startsWith('data: ')) {
    onChunk(remainder.slice(6));
  }
}

/**
 * Parse raw SSE data string into a typed object.
 * Returns undefined if the data is not valid JSON or lacks a type field.
 */
export function parseSSERaw(
  rawData: string,
): { type: string; data: Record<string, unknown> } | undefined {
  try {
    const parsed = typeof rawData === 'string' ? JSON.parse(rawData) : undefined;
    if (parsed && typeof parsed.type === 'string' && parsed.data && typeof parsed.data === 'object') {
      return { type: parsed.type, data: parsed.data as Record<string, unknown> };
    }
    return undefined;
  } catch {
    return undefined;
  }
}