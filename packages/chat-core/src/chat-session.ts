import type { CopConMessage, ChatSessionCallbacks, SessionState } from './types';
import type { AgentClient } from './agent-client';
import { applySSEChunk, createUserMessage, mergeMessages } from './message-reducer';
import { parseSSEStream } from './sse-parser';

export interface ChatSessionConfig {
  client: AgentClient;
  sessionId: string;
  callbacks: ChatSessionCallbacks;
}

export class ChatSession {
  private messages: CopConMessage[] = [];
  private currentAssistantIndex: number | null = null;
  private seq = 0;
  private abortController: AbortController | null = null;
  private isRequesting = false;
  private isReconnecting = false;
  private isStreamComplete = false;
  private config: ChatSessionConfig;

  constructor(config: ChatSessionConfig) {
    this.config = config;
  }

  async start(): Promise<void> {
    await this.loadMessages();
    this.connectStream();
  }

  sendMessage(content: string): void {
    const userMsg = createUserMessage(content);
    this.messages.push(userMsg);
    this.config.callbacks.onMessagesChange([...this.messages]);
    this.connectStream(content);
    this.updateState('streaming');
  }

  abort(): void {
    if (this.abortController) {
      this.abortController.abort();
    }
    this.isRequesting = false;
    this.updateState('idle');
  }

  async loadMessages(): Promise<void> {
    const result = await this.config.client.getMessages(this.config.sessionId);
    this.messages = result.messages || [];
    this.config.callbacks.onMessagesChange([...this.messages]);
  }

  destroy(): void {
    if (this.abortController) {
      this.abortController.abort();
    }
    this.abortController = null;
    this.isRequesting = false;
    this.isReconnecting = false;
    this.messages = [];
    this.currentAssistantIndex = null;
    this.seq = 0;
  }

  private async connectStream(content?: string): Promise<void> {
    this.abortController = new AbortController();
    this.isRequesting = true;
    this.isStreamComplete = false;

    const body: Record<string, unknown> = {
      content: content || '',
      sessionId: this.config.sessionId,
    };

    if (!content && this.messages.some(m => m.role === 'assistant')) {
      body.reconnect = true;
      body.last_event_seq = this.seq;
    }

    try {
      const response = await fetch(
        `${this.config.client.getBaseUrl()}/api/sessions/${this.config.sessionId}/chat`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
          signal: this.abortController.signal,
        },
      );

      if (!response.ok) {
        throw new Error(`Chat request failed: ${response.statusText}`);
      }

      if (!response.body) {
        throw new Error('No response body');
      }

      const reader = response.body.getReader();

      await parseSSEStream(reader, (rawData) => {
        this.seq++;

        try {
          const parsed = JSON.parse(rawData);
          if (parsed.type === 'message_done') {
            this.isStreamComplete = true;
          }
        } catch {}

        if (this.currentAssistantIndex === null) {
          const newMsg: CopConMessage = {
            id: `assistant-${Date.now()}`,
            role: 'assistant',
            steps: [],
            metadata: { createdAt: new Date().toISOString() },
          };
          this.messages.push(newMsg);
          this.currentAssistantIndex = this.messages.length - 1;
        }

        const updated = applySSEChunk(this.messages[this.currentAssistantIndex!], rawData);
        this.messages[this.currentAssistantIndex!] = updated;

        if (this.isStreamComplete) {
          this.currentAssistantIndex = null;
        }

        this.config.callbacks.onMessagesChange([...this.messages]);
      });
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        return;
      }
      await this.handleReconnect();
    } finally {
      this.isRequesting = false;
      this.updateState('idle');
    }
  }

  private async handleReconnect(): Promise<void> {
    if (this.isReconnecting) return;
    this.isReconnecting = true;
    this.updateState('reconnecting');

    try {
      const response = await this.config.client.reconnect(
        this.config.sessionId,
        this.seq + 1,
      );

      if (response.status === 204 || !response.body) {
        await this.loadMessages();
        return;
      }

      let eventsLost = false;
      const reader = response.body.getReader();

      await parseSSEStream(reader, (rawData) => {
        this.seq++;

        try {
          const parsed = JSON.parse(rawData);
          if (parsed.type === 'events_lost') {
            eventsLost = true;
            reader.cancel();
            return;
          }
          if (parsed.type === 'message_done') {
            this.isStreamComplete = true;
          }
        } catch {}

        if (this.currentAssistantIndex === null) {
          const newMsg: CopConMessage = {
            id: `assistant-${Date.now()}`,
            role: 'assistant',
            steps: [],
            metadata: { createdAt: new Date().toISOString() },
          };
          this.messages.push(newMsg);
          this.currentAssistantIndex = this.messages.length - 1;
        }

        const updated = applySSEChunk(this.messages[this.currentAssistantIndex!], rawData);
        this.messages[this.currentAssistantIndex!] = updated;

        if (this.isStreamComplete) {
          this.currentAssistantIndex = null;
        }

        this.config.callbacks.onMessagesChange([...this.messages]);
      });

      if (eventsLost) {
        const localMessages = [...this.messages];
        await this.loadMessages();
        this.messages = mergeMessages(this.messages, localMessages);
        this.config.callbacks.onMessagesChange([...this.messages]);
        return;
      }
    } catch {
      try {
        const localMessages = [...this.messages];
        await this.loadMessages();
        this.messages = mergeMessages(this.messages, localMessages);
        this.config.callbacks.onMessagesChange([...this.messages]);
      } catch {}
      this.seq = 0;
      this.connectStream();
    } finally {
      this.isReconnecting = false;
      this.updateState(this.isRequesting ? 'streaming' : 'idle');
    }
  }

  private updateState(status: SessionState['status']): void {
    this.config.callbacks.onStateChange({ status, error: undefined });
  }
}
