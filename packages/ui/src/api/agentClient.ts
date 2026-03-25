import { Session, Message, SSEEvent } from './types';

export interface AgentClientConfig {
  baseUrl: string;
}

export class AgentClient {
  private baseUrl: string;

  constructor(config: AgentClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
  }

  async createSession(): Promise<Session> {
    const response = await fetch(`${this.baseUrl}/api/sessions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!response.ok) throw new Error(`Failed to create session: ${response.statusText}`);
    return response.json();
  }

  async getSessions(): Promise<{ sessions: Session[]; total: number }> {
    const response = await fetch(`${this.baseUrl}/api/sessions`);
    if (!response.ok) throw new Error(`Failed to get sessions: ${response.statusText}`);
    return response.json();
  }

  async getSession(sessionId: string): Promise<Session> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}`);
    if (!response.ok) throw new Error(`Failed to get session: ${response.statusText}`);
    return response.json();
  }

  async deleteSession(sessionId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}`, {
      method: 'DELETE',
    });
    if (!response.ok) throw new Error(`Failed to delete session: ${response.statusText}`);
  }

  async getMessages(sessionId: string): Promise<{ messages: Message[] }> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/messages`);
    if (!response.ok) throw new Error(`Failed to get messages: ${response.statusText}`);
    return response.json();
  }

  chat(
    sessionId: string,
    content: string,
    onEvent: (event: SSEEvent) => void,
    onError?: (error: Error) => void,
    onComplete?: () => void
  ): () => void {
    const controller = new AbortController();
    
    fetch(`${this.baseUrl}/api/sessions/${sessionId}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content }),
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`Chat request failed: ${response.statusText}`);
        }
        
        const reader = response.body?.getReader();
        if (!reader) throw new Error('No response body');
        
        const decoder = new TextDecoder();
        let buffer = '';
        
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n\n');
          buffer = lines.pop() || '';
          
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const data = line.slice(6);
              if (data === '[DONE]') {
                onComplete?.();
                return;
              }
              try {
                const event = JSON.parse(data) as SSEEvent;
                console.log('[AgentClient] Parsed SSE event:', event);
                onEvent(event);
              } catch (e) {
                console.error('[AgentClient] Failed to parse SSE data:', data, e);
              }
            } else if (line.startsWith('event: ')) {
              // Handle event type line
            }
          }
        }
      })
      .catch((error) => {
        if (error.name !== 'AbortError') {
          onError?.(error);
        }
      });
    
    return () => controller.abort();
  }
}