import type { CopConMessage, SessionState } from '@copcon/chat-core';

interface ChatStateSnapshot {
  messages: CopConMessage[];
  state: SessionState;
}

export class ReactChatState {
  private messages: CopConMessage[] = [];
  private state: SessionState = { status: 'idle', error: undefined };
  private listeners = new Set<() => void>();

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  getSnapshot(): ChatStateSnapshot {
    return { messages: this.messages, state: this.state };
  }

  setMessages(messages: CopConMessage[]): void {
    this.messages = messages;
    this.emitChange();
  }

  setState(state: SessionState): void {
    this.state = state;
    this.emitChange();
  }

  private emitChange(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}