import type { ToolCallPart, InterruptPayload } from '@copcon/chat-core';
import { parseToolOutput } from '@copcon/chat-core';

interface ToolCallCallbacks {
  onApprove?: (interrupt: InterruptPayload) => void;
  onDecline?: (interrupt: InterruptPayload) => void;
}

interface ToolCallController {
  toolName: string;
  status: string;
  parsedArgs: unknown;
  parsedOutput: { status: 'success' | 'error'; output: string };
  error: string;
  needsApproval: boolean;
  interrupt: InterruptPayload | undefined;
  approve: () => void;
  decline: () => void;
  getStatusProps: () => { 'aria-live': string };
}

export function createToolCallController(
  part: ToolCallPart,
  callbacks?: ToolCallCallbacks,
): ToolCallController {
  let parsedArgs: unknown;
  try {
    parsedArgs = JSON.parse(part.args);
  } catch {
    parsedArgs = part.args;
  }

  const needsApproval = part.state === 'waiting_for_input';

  return {
    toolName: part.toolName,
    status: part.state,
    parsedArgs,
    parsedOutput: parseToolOutput(part.output),
    error: part.error,
    needsApproval,
    interrupt: part.interrupt,
    approve: () => {
      if (part.interrupt && callbacks?.onApprove) {
        callbacks.onApprove(part.interrupt);
      }
    },
    decline: () => {
      if (part.interrupt && callbacks?.onDecline) {
        callbacks.onDecline(part.interrupt);
      }
    },
    getStatusProps: () => ({ 'aria-live': 'polite' }),
  };
}