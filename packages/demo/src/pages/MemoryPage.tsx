import React, { useState, useEffect, useCallback } from 'react';
import {
  Flex,
  Select,
  Card,
  List,
  Typography,
  theme,
  Skeleton,
  Empty,
  Button,
  Popconfirm,
  Tag,
  message,
} from 'antd';
import { DatabaseOutlined, DeleteOutlined } from '@ant-design/icons';
import { XMarkdown } from '@ant-design/x-markdown';
import { useClient } from '../context/ClientContext';
import type { Agent, Memory } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

const MEMORY_TYPE_COLORS: Record<string, string> = {
  episodic: 'blue',
  semantic: 'green',
  procedural: 'purple',
};

const MemoryPage: React.FC = () => {
  const { token } = useToken();
  const client = useClient();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [selectedAgentId, setSelectedAgentId] = useState<string>('');
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loadingMemories, setLoadingMemories] = useState(false);

  const loadAgents = useCallback(async () => {
    setLoadingAgents(true);
    try {
      const result = await client.getAgents();
      const list = result.agents || [];
      setAgents(list);
      if (list.length > 0 && !selectedAgentId) {
        setSelectedAgentId(list[0].id);
      }
    } catch {
      message.error('Failed to load agents');
    } finally {
      setLoadingAgents(false);
    }
  }, [client, selectedAgentId]);

  const loadMemories = useCallback(async () => {
    if (!selectedAgentId) return;
    setLoadingMemories(true);
    try {
      const result = await client.getAgentMemories(selectedAgentId);
      setMemories(result.memories || []);
    } catch {
      setMemories([]);
    } finally {
      setLoadingMemories(false);
    }
  }, [client, selectedAgentId]);

  useEffect(() => {
    loadAgents();
  }, [loadAgents]);

  useEffect(() => {
    loadMemories();
  }, [loadMemories]);

  const handleDelete = async (memoryId: string) => {
    try {
      await client.deleteAgentMemory(selectedAgentId, memoryId);
      setMemories((prev) => prev.filter((m) => m.id !== memoryId));
      message.success('Memory deleted');
    } catch {
      message.error('Failed to delete memory');
    }
  };

  const agentOptions = agents.map((a) => ({
    value: a.id,
    label: a.name,
  }));

  return (
    <Flex vertical style={{ height: '100%', padding: token.padding, overflow: 'auto' }}>
      <Flex justify="space-between" align="center" style={{ marginBottom: token.margin }}>
        <Text strong style={{ fontSize: 18 }}>
          Memory Management
        </Text>
        <Select
          value={selectedAgentId || undefined}
          onChange={setSelectedAgentId}
          placeholder="Select an agent"
          options={agentOptions}
          loading={loadingAgents}
          style={{ width: 280 }}
          aria-label="Select agent"
        />
      </Flex>

      {loadingMemories ? (
        <Flex vertical gap="middle">
          {[1, 2, 3, 4].map((i) => (
            <Skeleton key={i} active paragraph={{ rows: 3 }} />
          ))}
        </Flex>
      ) : memories.length > 0 ? (
        <List
          grid={{ gutter: 16, column: 2 }}
          dataSource={memories}
          renderItem={(memory) => (
            <List.Item>
              <Card
                size="small"
                style={{ width: '100%' }}
                title={
                  <Flex justify="space-between" align="center">
                    <Tag color={MEMORY_TYPE_COLORS[memory.memory_type] || 'default'}>
                      {memory.memory_type}
                    </Tag>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {new Date(memory.timestamp).toLocaleString()}
                    </Text>
                  </Flex>
                }
                extra={
                  <Popconfirm
                    title="Delete memory?"
                    onConfirm={() => handleDelete(memory.id)}
                    okText="Delete"
                    cancelText="Cancel"
                    okButtonProps={{ danger: true }}
                  >
                    <Button
                      type="text"
                      size="small"
                      danger
                      icon={<DeleteOutlined />}
                      aria-label={`Delete memory ${memory.id.slice(0, 8)}`}
                    />
                  </Popconfirm>
                }
              >
                <div style={{ maxHeight: 200, overflow: 'auto' }}>
                  <XMarkdown content={memory.content} />
                </div>
                <Flex justify="space-between" style={{ marginTop: token.marginSM }}>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    Score: {memory.score.toFixed(3)}
                  </Text>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    Importance: {memory.importance.toFixed(2)}
                  </Text>
                </Flex>
              </Card>
            </List.Item>
          )}
        />
      ) : (
        <Flex flex={1} justify="center" align="center">
          <Empty
            image={<DatabaseOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
            description={
              <Flex vertical gap="small" align="center">
                <Text type="secondary">No memories for this agent</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  Memories are created automatically during conversations
                </Text>
              </Flex>
            }
          />
        </Flex>
      )}
    </Flex>
  );
};

export default MemoryPage;
