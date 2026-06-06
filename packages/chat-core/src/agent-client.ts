import type { Session, Agent, CopConMessage, Todo, KnowledgeBase, Document, Chunk, SearchResult, Memory, SkillInfo, SkillDetail, MCPServerInfo, MCPServerConfig } from './types';

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

  async getAgents(): Promise<{ agents: Agent[] }> {
    const response = await fetch(`${this.baseUrl}/api/agents`);
    if (!response.ok) throw new Error(`Failed to get agents: ${response.statusText}`);
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

  async getMessages(sessionId: string): Promise<{ messages: CopConMessage[] }> {
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

  async listKnowledgeBases(): Promise<{ knowledge_bases: KnowledgeBase[] }> {
    const response = await fetch(`${this.baseUrl}/api/kb`);
    if (!response.ok) throw new Error(`Failed to list knowledge bases: ${response.statusText}`);
    return response.json();
  }

  async createKnowledgeBase(name: string, backend?: string, config?: Record<string, unknown>): Promise<KnowledgeBase> {
    const body: Record<string, unknown> = { name };
    if (backend) body.backend = backend;
    if (config) body.config = config;
    const response = await fetch(`${this.baseUrl}/api/kb`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`Failed to create knowledge base: ${response.statusText}`);
    return response.json();
  }

  async deleteKnowledgeBase(kbId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}`, {
      method: 'DELETE',
    });
    if (!response.ok) throw new Error(`Failed to delete knowledge base: ${response.statusText}`);
  }

  async listDocuments(kbId: string): Promise<{ documents: Document[] }> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs`);
    if (!response.ok) throw new Error(`Failed to list documents: ${response.statusText}`);
    return response.json();
  }

  async uploadDocument(kbId: string, file: File): Promise<Document> {
    const formData = new FormData();
    formData.append('file', file);
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs`, {
      method: 'POST',
      body: formData,
    });
    if (!response.ok) throw new Error(`Failed to upload document: ${response.statusText}`);
    return response.json();
  }

  async uploadText(kbId: string, filename: string, content: string): Promise<Document> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/text`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename, content }),
    });
    if (!response.ok) throw new Error(`Failed to upload text: ${response.statusText}`);
    return response.json();
  }

  async deleteDocument(kbId: string, docId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/${docId}`, {
      method: 'DELETE',
    });
    if (!response.ok) throw new Error(`Failed to delete document: ${response.statusText}`);
  }

  async getDocumentChunks(kbId: string, docId: string): Promise<{ chunks: Chunk[] }> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/${docId}/chunks`);
    if (!response.ok) throw new Error(`Failed to get document chunks: ${response.statusText}`);
    return response.json();
  }

  async getDocumentContent(kbId: string, docId: string): Promise<Document> {
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/docs/${docId}?include_content=true`);
    if (!response.ok) throw new Error(`Failed to get document content: ${response.statusText}`);
    return response.json();
  }

  async testRetrieval(kbId: string, query: string, topK?: number, similarityThreshold?: number): Promise<SearchResult> {
    const body: Record<string, unknown> = { query };
    if (topK !== undefined) body.top_k = topK;
    if (similarityThreshold !== undefined) body.similarity_threshold = similarityThreshold;
    const response = await fetch(`${this.baseUrl}/api/kb/${kbId}/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`Failed to test retrieval: ${response.statusText}`);
    return response.json();
  }

  async getAgentMemories(agentId: string, limit?: number): Promise<{ memories: Memory[] }> {
    const url = limit
      ? `${this.baseUrl}/api/agents/${agentId}/memories?limit=${limit}`
      : `${this.baseUrl}/api/agents/${agentId}/memories`;
    const response = await fetch(url);
    if (!response.ok) throw new Error(`Failed to get agent memories: ${response.statusText}`);
    return response.json();
  }

  async deleteAgentMemory(agentId: string, memoryId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/agents/${agentId}/memories/${memoryId}`, {
      method: 'DELETE',
    });
    if (!response.ok) throw new Error(`Failed to delete agent memory: ${response.statusText}`);
  }

  async listSkills(): Promise<{ skills: SkillInfo[] }> {
    const response = await fetch(`${this.baseUrl}/api/skills`);
    if (!response.ok) throw new Error(`Failed to list skills: ${response.statusText}`);
    return response.json();
  }

  async getSkill(name: string, includeContent?: boolean): Promise<SkillDetail> {
    const url = includeContent
      ? `${this.baseUrl}/api/skills/${name}?include_content=true`
      : `${this.baseUrl}/api/skills/${name}`;
    const response = await fetch(url);
    if (!response.ok) throw new Error(`Failed to get skill: ${response.statusText}`);
    return response.json();
  }

  async enableSkill(name: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/skills/${name}/enable`, {
      method: 'POST',
    });
    if (!response.ok) throw new Error(`Failed to enable skill: ${response.statusText}`);
  }

  async disableSkill(name: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/skills/${name}/disable`, {
      method: 'POST',
    });
    if (!response.ok) throw new Error(`Failed to disable skill: ${response.statusText}`);
  }

  async listMCPServers(): Promise<{ servers: MCPServerInfo[] }> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers`);
    if (!response.ok) throw new Error(`Failed to list MCP servers: ${response.statusText}`);
    return response.json();
  }

  async getMCPServer(name: string): Promise<MCPServerInfo> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers/${name}`);
    if (!response.ok) throw new Error(`Failed to get MCP server: ${response.statusText}`);
    return response.json();
  }

  async addMCPServer(config: MCPServerConfig): Promise<MCPServerInfo> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
    if (!response.ok) throw new Error(`Failed to add MCP server: ${response.statusText}`);
    return response.json();
  }

  async removeMCPServer(name: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers/${name}`, {
      method: 'DELETE',
    });
    if (!response.ok) throw new Error(`Failed to remove MCP server: ${response.statusText}`);
  }

  async enableMCPServer(name: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers/${name}/enable`, {
      method: 'POST',
    });
    if (!response.ok) throw new Error(`Failed to enable MCP server: ${response.statusText}`);
  }

  async disableMCPServer(name: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/api/mcp/servers/${name}/disable`, {
      method: 'POST',
    });
    if (!response.ok) throw new Error(`Failed to disable MCP server: ${response.statusText}`);
  }
}