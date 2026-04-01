import type { CopConMessage } from '../providers/CopConChatProvider';

/**
 * Parsed tool output structure
 */
interface ParsedToolOutput {
  status: 'success' | 'error';
  output: string;
}

/**
 * Parses tool output content to extract status and display message.
 * 
 * Expected JSON format:
 * - Success: { success: true, data: { message: "..." } }
 * - Error: { success: false, error: "..." }
 *
 * @param rawOutput - Raw output string from tool execution
 * @returns Parsed output with status and display message
 */
export function parseToolOutput(rawOutput: string): ParsedToolOutput {
  if (!rawOutput || rawOutput.trim() === '') {
    return { status: 'success', output: '' };
  }

  try {
    const parsed = JSON.parse(rawOutput);
    
    if (typeof parsed.success === 'boolean') {
      if (!parsed.success) {
        return {
          status: 'error',
          output: parsed.error || 'Unknown error',
        };
      }
      const dataMessage = parsed.data?.message || parsed.data?.response;
      return {
        status: 'success',
        output: dataMessage || parsed.message || rawOutput,
      };
    }
    
    return { status: 'success', output: rawOutput };
  } catch {
    return { status: 'success', output: rawOutput };
  }
}

/**
 * Merges tool result messages into their parent assistant messages' tool_calls array.
 * Tool results (role=tool) are matched to tool_calls by tool_call_id.
 *
 * @param messages - Array of messages to merge
 * @returns New array with tool results merged into assistant messages
 */
export function mergeToolMessages(messages: CopConMessage[]): CopConMessage[] {
  const toolMessages = new Map<string, CopConMessage>();
  messages.forEach((msg) => {
    if (msg.role === 'tool' && msg.tool_call_id) {
      toolMessages.set(msg.tool_call_id, msg);
    }
  });

  const result: CopConMessage[] = [];
  messages.forEach((msg) => {
    if (msg.role === 'tool') {
      return;
    }

    if (msg.role === 'assistant' && msg.tool_calls) {
      const mergedToolCalls = msg.tool_calls.map((tc) => {
        const toolMsg = toolMessages.get(tc.id);
        if (toolMsg) {
          const { status, output } = parseToolOutput(toolMsg.content);
          return {
            ...tc,
            status,
            output,
          };
        }
        return tc;
      });
      result.push({ ...msg, tool_calls: mergedToolCalls });
    } else {
      result.push(msg);
    }
  });

  return result;
}
