import type { CopConMessage, SessionState } from '@copcon/chat-core';

interface ChatStateSnapshot {
  messages: CopConMessage[];
  state: SessionState;
}

export class ReactChatState {
  private messages: CopConMessage[] = [];
  private state: SessionState = { status: 'idle', error: undefined };
  private listeners = new Set<() => void>();
  private cachedSnapshot: ChatStateSnapshot | null = null;
  private snapshotDirty = false;

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  getSnapshot(): ChatStateSnapshot {
    if (this.snapshotDirty || this.cachedSnapshot === null) {
      this.cachedSnapshot = { messages: this.messages, state: this.state };
      this.snapshotDirty = false;
    }
    return this.cachedSnapshot;
  }

  setMessages(messages: CopConMessage[]): void {
    if (this.messages === messages) return;
    this.messages = messages;
    this.snapshotDirty = true;
    this.emitChange();
  }

  setState(state: SessionState): void {
    if (this.state.status === state.status && this.state.error === state.error) return;
    this.state = state;
    this.snapshotDirty = true;
    this.emitChange();
  }

  private emitChange(): void {
    for (const listener of this.listeners) {
      listener();
    }
  }
}