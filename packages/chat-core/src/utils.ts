/**
 * Parsed tool output structure
 */
export interface ParsedToolOutput {
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