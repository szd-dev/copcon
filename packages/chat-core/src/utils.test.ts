import { describe, it, expect } from 'vitest';
import { parseToolOutput } from './utils';

describe('parseToolOutput', () => {
  it('parses success JSON with data.message', () => {
    const result = parseToolOutput('{"success":true,"data":{"message":"Done"}}');
    expect(result).toEqual({ status: 'success', output: 'Done' });
  });

  it('parses error JSON', () => {
    const result = parseToolOutput('{"success":false,"error":"Something failed"}');
    expect(result).toEqual({ status: 'error', output: 'Something failed' });
  });

  it('returns raw string for plain text input', () => {
    const result = parseToolOutput('plain text output');
    expect(result).toEqual({ status: 'success', output: 'plain text output' });
  });

  it('handles empty string', () => {
    const result = parseToolOutput('');
    expect(result).toEqual({ status: 'success', output: '' });
  });

  it('parses success JSON with data.response', () => {
    const result = parseToolOutput('{"success":true,"data":{"response":"Response text"}}');
    expect(result).toEqual({ status: 'success', output: 'Response text' });
  });

  it('falls back to raw output when no data.message or data.response', () => {
    const raw = '{"success":true,"data":{}}';
    const result = parseToolOutput(raw);
    expect(result).toEqual({ status: 'success', output: raw });
  });

  it('returns Unknown error when success is false without error field', () => {
    const result = parseToolOutput('{"success":false}');
    expect(result).toEqual({ status: 'error', output: 'Unknown error' });
  });
});
