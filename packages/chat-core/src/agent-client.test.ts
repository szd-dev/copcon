import { describe, it, expect, vi, beforeEach } from 'vitest';
import { AgentClient } from './agent-client';
import type { KnowledgeBase, Document, SearchResult, Memory } from './types';

function mockFetch(response: unknown, status = 200, statusText = 'OK') {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText,
    json: () => Promise.resolve(response),
  });
}

describe('AgentClient — Knowledge Base Methods', () => {
  let client: AgentClient;

  beforeEach(() => {
    client = new AgentClient({ baseUrl: 'http://localhost:8080' });
    vi.restoreAllMocks();
  });

  describe('listKnowledgeBases', () => {
    it('calls GET /api/kb and returns knowledge bases', async () => {
      const kbs: KnowledgeBase[] = [
        { id: 'kb1', name: 'Test KB', backend: 'sqlite-vec', config: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', metadata: {} },
      ];
      globalThis.fetch = mockFetch({ knowledge_bases: kbs });

      const result = await client.listKnowledgeBases();
      expect(result.knowledge_bases).toHaveLength(1);
      expect(result.knowledge_bases[0].id).toBe('kb1');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb');
    });

    it('throws on server error', async () => {
      globalThis.fetch = mockFetch(null, 500, 'Internal Server Error');
      await expect(client.listKnowledgeBases()).rejects.toThrow('Failed to list knowledge bases');
    });
  });

  describe('createKnowledgeBase', () => {
    it('calls POST /api/kb with name and backend', async () => {
      const kb: KnowledgeBase = { id: 'kb1', name: 'New KB', backend: 'sqlite-vec', config: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', metadata: {} };
      globalThis.fetch = mockFetch(kb, 201);

      const result = await client.createKnowledgeBase('New KB', 'sqlite-vec');
      expect(result.name).toBe('New KB');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb', expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'New KB', backend: 'sqlite-vec' }),
      }));
    });

    it('omits backend and config when not provided', async () => {
      const kb: KnowledgeBase = { id: 'kb2', name: 'Simple', backend: 'sqlite-vec', config: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', metadata: {} };
      globalThis.fetch = mockFetch(kb, 201);

      await client.createKnowledgeBase('Simple');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb', expect.objectContaining({
        body: JSON.stringify({ name: 'Simple' }),
      }));
    });
  });

  describe('deleteKnowledgeBase', () => {
    it('calls DELETE /api/kb/:kbId', async () => {
      globalThis.fetch = mockFetch(null, 204);

      await client.deleteKnowledgeBase('kb1');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1', expect.objectContaining({
        method: 'DELETE',
      }));
    });

    it('throws on 404', async () => {
      globalThis.fetch = mockFetch(null, 404, 'Not Found');
      await expect(client.deleteKnowledgeBase('nonexistent')).rejects.toThrow('Failed to delete knowledge base');
    });
  });

  describe('listDocuments', () => {
    it('calls GET /api/kb/:kbId/docs', async () => {
      const docs: Document[] = [
        { id: 'd1', kb_id: 'kb1', filename: 'test.txt', source: 'upload', status: 'ready', chunk_count: 3, token_count: 100, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', metadata: {} },
      ];
      globalThis.fetch = mockFetch({ documents: docs });

      const result = await client.listDocuments('kb1');
      expect(result.documents).toHaveLength(1);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1/docs');
    });
  });

  describe('uploadDocument', () => {
    it('calls POST /api/kb/:kbId/docs with FormData', async () => {
      const doc: Document = { id: 'd1', kb_id: 'kb1', filename: 'test.txt', source: 'upload', status: 'pending', chunk_count: 0, token_count: 0, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', metadata: {} };
      globalThis.fetch = mockFetch(doc, 202);

      const file = new File(['hello world'], 'test.txt', { type: 'text/plain' });
      const result = await client.uploadDocument('kb1', file);
      expect(result.filename).toBe('test.txt');

      const callArgs = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(callArgs[1].method).toBe('POST');
      expect(callArgs[1].body).toBeInstanceOf(FormData);
      expect(callArgs[1].headers).toBeUndefined();
    });
  });

  describe('deleteDocument', () => {
    it('calls DELETE /api/kb/:kbId/docs/:docId', async () => {
      globalThis.fetch = mockFetch(null, 204);

      await client.deleteDocument('kb1', 'd1');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1/docs/d1', expect.objectContaining({
        method: 'DELETE',
      }));
    });

    it('throws on 404', async () => {
      globalThis.fetch = mockFetch(null, 404, 'Not Found');
      await expect(client.deleteDocument('kb1', 'nonexistent')).rejects.toThrow('Failed to delete document');
    });
  });

  describe('getDocumentChunks', () => {
    it('calls GET /api/kb/:kbId/docs/:docId/chunks', async () => {
      const chunks = [
        { id: 'c1', document_id: 'd1', kb_id: 'kb1', content: 'chunk text', context: '', index: 0, token_count: 10, metadata: {}, score: 0.95 },
      ];
      globalThis.fetch = mockFetch({ chunks });

      const result = await client.getDocumentChunks('kb1', 'd1');
      expect(result.chunks).toHaveLength(1);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1/docs/d1/chunks');
    });
  });

  describe('testRetrieval', () => {
    it('calls POST /api/kb/:kbId/search with query', async () => {
      const searchResult: SearchResult = {
        results: [{ id: 'c1', document_id: 'd1', kb_id: 'kb1', content: 'match', context: '', index: 0, token_count: 10, metadata: {}, score: 0.9 }],
      };
      globalThis.fetch = mockFetch(searchResult);

      const result = await client.testRetrieval('kb1', 'test query');
      expect(result.results).toHaveLength(1);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1/search', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ query: 'test query' }),
      }));
    });

    it('includes optional topK and similarityThreshold', async () => {
      globalThis.fetch = mockFetch({ results: [] });

      await client.testRetrieval('kb1', 'query', 10, 0.5);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/kb/kb1/search', expect.objectContaining({
        body: JSON.stringify({ query: 'query', top_k: 10, similarity_threshold: 0.5 }),
      }));
    });
  });

  describe('getSessionMemories', () => {
    it('calls GET /api/sessions/:sessionId/memories', async () => {
      const memories: Memory[] = [
        { id: 'm1', content: 'memory 1', session_id: 's1', role: 'assistant', timestamp: '2026-01-01T00:00:00Z', memory_type: 'episodic', metadata: {}, score: 0, importance: 0.5 },
      ];
      globalThis.fetch = mockFetch({ memories });

      const result = await client.getSessionMemories('s1');
      expect(result.memories).toHaveLength(1);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/sessions/s1/memories');
    });

    it('includes limit query parameter when provided', async () => {
      globalThis.fetch = mockFetch({ memories: [] });

      await client.getSessionMemories('s1', 10);
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/sessions/s1/memories?limit=10');
    });
  });

  describe('deleteSessionMemory', () => {
    it('calls DELETE /api/sessions/:sessionId/memories/:memoryId', async () => {
      globalThis.fetch = mockFetch(null, 204);

      await client.deleteSessionMemory('s1', 'm1');
      expect(globalThis.fetch).toHaveBeenCalledWith('http://localhost:8080/api/sessions/s1/memories/m1', expect.objectContaining({
        method: 'DELETE',
      }));
    });

    it('throws on 404', async () => {
      globalThis.fetch = mockFetch(null, 404, 'Not Found');
      await expect(client.deleteSessionMemory('s1', 'nonexistent')).rejects.toThrow('Failed to delete session memory');
    });
  });
});
