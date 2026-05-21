import { Session, Message, Todo } from './types';

export interface AgentClientConfig {
  baseUrl: string;
}

export class AgentClient {
  private baseUrl: string;

  constructor(config: AgentClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
  }

  getBaseUrl(): string {
    return this.baseUrl;
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

  async getTodos(sessionId: string): Promise<{ todos: Todo[] }> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/todos`);
    if (!response.ok) throw new Error(`Failed to get todos: ${response.statusText}`);
    return response.json();
  }

  async getUpdates(
    sessionId: string,
    since?: string
  ): Promise<{
    has_updates: boolean;
    events: Array<{
      id: string;
      call_id: string;
      tool_name: string;
      session_id: string;
      completed_at: string;
      status: string;
      error?: string;
    }>;
  }> {
    const url = since
      ? `${this.baseUrl}/api/sessions/${sessionId}/updates?since=${since}`
      : `${this.baseUrl}/api/sessions/${sessionId}/updates`;
    const response = await fetch(url);
    if (!response.ok) throw new Error(`Failed to get updates: ${response.statusText}`);
    return response.json();
  }

  /**
   * Reconnect to an existing SSE chat stream.
   *
   * Sends a POST to /api/sessions/{sessionId}/chat with reconnect=true and the
   * last known event sequence number. The backend will resume streaming events
   * from that point forward.
   *
   * NOTE: Uses fetch directly rather than XRequest because XRequest always
   * constructs the POST body from its `params` option (see @ant-design/x-sdk
   * source: body = JSON.stringify({...params, ...extraParams})). There is no
   * way to pass a custom body like { reconnect, last_event_seq } through
   * XRequestOptions without type changes.
   *
   * @param sessionId - The session to reconnect to.
   * @param lastEventSeq - The last received event sequence number.
   * @returns The raw Response whose body is an SSE stream.
   */
  async reconnect(sessionId: string, lastEventSeq: number): Promise<Response> {
    const response = await fetch(
      `${this.baseUrl}/api/sessions/${sessionId}/chat`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reconnect: true, last_event_seq: lastEventSeq }),
      },
    );
    if (!response.ok) {
      throw new Error(`Failed to reconnect: ${response.statusText}`);
    }
    return response;
  }

  /**
   * Resume a session after a human-in-the-loop interrupt.
   *
   * Sends the user's decision (approve/decline/submit/cancel) to the backend,
   * optionally with form data (content). The backend will unblock the agent
   * loop and continue processing the interrupted tool call.
   *
   * @param sessionId - The session to resume.
   * @param interruptId - The interrupt to respond to.
   * @param action - The user's decision.
   * @param content - Optional structured data from a form response.
   */
  async resume(
    sessionId: string,
    interruptId: string,
    action: 'approve' | 'decline' | 'submit' | 'cancel',
    content?: Record<string, unknown>,
  ): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/resume`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ interrupt_id: interruptId, action, content }),
    });
    if (!response.ok) {
      throw new Error(`Failed to resume: ${response.statusText}`);
    }
  }

  /**
   * Stop a running session.
   *
   * Sends a POST to /api/sessions/{sessionId}/stop. The backend will abort
   * the current agent loop and mark the session as stopped.
   *
   * @param sessionId - The session to stop.
   */
  async stop(sessionId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/sessions/${sessionId}/stop`, {
      method: 'POST',
    });
    if (!response.ok) {
      throw new Error(`Failed to stop: ${response.statusText}`);
    }
  }
}