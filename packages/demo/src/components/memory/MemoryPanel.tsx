import React, { useState, useEffect, useCallback } from 'react';
import { Flex, List, Typography, theme, Skeleton, Empty, Button, Popconfirm, Tag, message } from 'antd';
import { DatabaseOutlined, DeleteOutlined } from '@ant-design/icons';
import { XMarkdown } from '@ant-design/x-markdown';
import { useClient } from '../../context/ClientContext';
import type { Memory } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

interface MemoryPanelProps {
  agentId: string;
  limit?: number;
}

const MEMORY_TYPE_COLORS: Record<string, string> = {
  episodic: 'blue',
  semantic: 'green',
  procedural: 'purple',
};

export const MemoryPanel: React.FC<MemoryPanelProps> = ({ agentId, limit = 5 }) => {
  const { token } = useToken();
  const client = useClient();
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);

  const loadMemories = useCallback(async () => {
    if (!agentId) return;
    setLoading(true);
    try {
      const result = await client.getAgentMemories(agentId, limit);
      setMemories(result.memories || []);
    } catch {
      setMemories([]);
    } finally {
      setLoading(false);
    }
  }, [client, agentId, limit]);

  useEffect(() => {
    loadMemories();
  }, [loadMemories]);

  const handleDelete = async (memoryId: string) => {
    try {
      await client.deleteAgentMemory(agentId, memoryId);
      setMemories((prev) => prev.filter((m) => m.id !== memoryId));
      message.success('Memory deleted');
    } catch {
      message.error('Failed to delete memory');
    }
  };

  if (loading) {
    return (
      <Flex vertical gap="small" style={{ padding: token.paddingSM }}>
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} active paragraph={{ rows: 2 }} />
        ))}
      </Flex>
    );
  }

  if (memories.length === 0) {
    return (
      <Flex justify="center" align="center" style={{ padding: token.paddingLG }}>
        <Empty
          image={<DatabaseOutlined style={{ fontSize: 32, color: token.colorTextDisabled }} />}
          description={<Text type="secondary">No memories</Text>}
        />
      </Flex>
    );
  }

  return (
    <List
      size="small"
      dataSource={memories}
      renderItem={(memory) => (
        <List.Item
          style={{
            padding: token.paddingSM,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
          }}
          actions={[
            <Popconfirm
              key="delete"
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
            </Popconfirm>,
          ]}
        >
          <Flex vertical gap="small" style={{ width: '100%' }}>
            <Flex justify="space-between" align="center">
              <Tag color={MEMORY_TYPE_COLORS[memory.memory_type] || 'default'}>
                {memory.memory_type}
              </Tag>
              <Text type="secondary" style={{ fontSize: 11 }}>
                {new Date(memory.timestamp).toLocaleString()}
              </Text>
            </Flex>
            <div style={{ fontSize: 13 }}>
              <XMarkdown content={memory.content} />
            </div>
          </Flex>
        </List.Item>
      )}
    />
  );
};
