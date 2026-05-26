import type { CopConMessage } from './types';
import type { AgentClient } from './agent-client';
import { applySSEChunk } from './message-reducer';
import { parseSSEStream } from './sse-parser';

export interface SubagentStreamConfig {
  client: AgentClient;
  sessionId: string;
  callbacks: {
    onMessagesChange: (messages: CopConMessage[]) => void;
    onStreamingChange: (isStreaming: boolean) => void;
    onError: (error: Error) => void;
  };
}

export class SubagentStream {
  private messages: CopConMessage[] = [];
  private currentMessage: CopConMessage | null = null;
  private seq: number = 0;
  private abortController: AbortController | null = null;
  private config: SubagentStreamConfig;

  constructor(config: SubagentStreamConfig) {
    this.config = config;
  }

  async start(): Promise<void> {
    this.abortController = new AbortController();
    this.config.callbacks.onStreamingChange(true);

    const { client, sessionId, callbacks } = this.config;

    try {
      const response = await fetch(
        `${client.getBaseUrl()}/api/sessions/${sessionId}/chat`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            reconnect: true,
            last_event_seq: 0,
            sessionId,
            content: '',
          }),
          signal: this.abortController.signal,
        },
      );

      if (!response.ok) {
        throw new Error(`Subagent stream failed: ${response.statusText}`);
      }

      const reader = response.body!.getReader();

      await parseSSEStream(reader, (rawData) => {
        this.seq++;

        if (this.currentMessage === null) {
          this.currentMessage = {
            id: `assistant-${Date.now()}`,
            role: 'assistant',
            steps: [],
            metadata: { createdAt: new Date().toISOString() },
          };
        }

        this.currentMessage = applySSEChunk(this.currentMessage, rawData);

        let isDone = false;
        try {
          const parsed = JSON.parse(rawData);
          if (parsed.type === 'message_done') {
            isDone = true;
          }
        } catch {}

        if (isDone) {
          this.messages = [...this.messages, this.currentMessage];
          this.currentMessage = null;
          callbacks.onStreamingChange(false);
        }

        callbacks.onMessagesChange([
          ...this.messages,
          ...(this.currentMessage ? [this.currentMessage] : []),
        ]);
      });

      callbacks.onStreamingChange(false);
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        return;
      }
      callbacks.onError(err instanceof Error ? err : new Error(String(err)));
      callbacks.onStreamingChange(false);
    }
  }

  destroy(): void {
    if (this.abortController) {
      this.abortController.abort();
      this.abortController = null;
    }
    this.config.callbacks.onStreamingChange(false);
  }
}
