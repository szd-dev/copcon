# Vector Memory Design

## Overview

This document describes the vector memory design for the Agent Infrastructure using Qdrant as the vector database.

## Collection Design

### Memory Collection

**Collection Name**: `agent_memory`

**Configuration**:
- **Vector Size**: 1536 (OpenAI text-embedding-ada-002)
- **Distance Metric**: Cosine
- **Quantization**: Scalar quantization (for memory efficiency)

**Payload Schema**:
```json
{
  "content": "string - The original text content",
  "session_id": "string - UUID of the session this memory belongs to",
  "role": "string - user/assistant/system",
  "timestamp": "integer - Unix timestamp",
  "memory_type": "string - conversation/summary/important",
  "metadata": "object - Additional metadata"
}
```

## Memory Types

### 1. Conversation Memory
- Stores important conversation turns
- Used for context retrieval in future sessions
- Triggered by: explicit user request, important decisions, or automatic importance detection

### 2. Summary Memory
- Stores summarized versions of long conversations
- Used to maintain context without storing all messages
- Generated periodically for long sessions

### 3. Important Memory
- Stores explicitly important information
- User preferences, key facts, decisions
- Higher priority in retrieval

## Retrieval Strategy

### Query Flow
1. User sends message
2. Generate embedding for user message
3. Search Qdrant for relevant memories
4. Inject relevant memories into context
5. LLM generates response

### Search Parameters
```yaml
default_search:
  limit: 5
  score_threshold: 0.7
  filters:
    session_id: current_session  # Prefer current session
  
cross_session_search:
  limit: 10
  score_threshold: 0.8
  filters:
    memory_type: important  # Only important memories from other sessions
```

## Embedding Strategy

- **Model**: text-embedding-ada-002
- **Dimension**: 1536
- **Batch Size**: 100 (for bulk operations)
- **Chunking**: 
  - Max chunk size: 8191 tokens (model limit)
  - Overlap: 200 tokens (for context continuity)

## Memory Lifecycle

### Creation
1. New message arrives
2. Determine if memory should be stored (importance detection)
3. Generate embedding
4. Store in Qdrant with metadata

### Update
- Memories are immutable by default
- Updates create new versions with timestamp

### Deletion
- Session deletion triggers cascade delete of session memories
- TTL-based cleanup for old memories (configurable)

## Performance Considerations

### Indexing
- HNSW index for fast approximate search
- Payload indexes on `session_id` and `memory_type`

### Caching
- Recent memories cached in memory (LRU cache)
- Cache size: 1000 vectors

### Batching
- Embedding generation batched for efficiency
- Async storage for non-blocking operation

## Configuration

```yaml
qdrant:
  host: localhost
  port: 6333
  collection: agent_memory
  
embedding:
  model: text-embedding-ada-002
  batch_size: 100
  
memory:
  importance_threshold: 0.7
  max_context_memories: 5
  ttl_days: 30
```