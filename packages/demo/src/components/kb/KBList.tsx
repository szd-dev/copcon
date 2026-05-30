import React from 'react';
import { Card, Flex, Skeleton, Empty, Button, theme, Typography, Popconfirm } from 'antd';
import { DeleteOutlined, DatabaseOutlined } from '@ant-design/icons';
import type { KnowledgeBase } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

interface KBListProps {
  knowledgeBases: KnowledgeBase[];
  loading: boolean;
  selectedId?: string;
  onSelect: (kb: KnowledgeBase) => void;
  onDelete: (kbId: string) => void;
  onCreate: () => void;
}

export const KBList: React.FC<KBListProps> = ({
  knowledgeBases,
  loading,
  selectedId,
  onSelect,
  onDelete,
  onCreate,
}) => {
  const { token } = useToken();

  if (loading) {
    return (
      <Flex vertical gap="middle" style={{ padding: token.padding }}>
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} active paragraph={{ rows: 2 }} />
        ))}
      </Flex>
    );
  }

  if (knowledgeBases.length === 0) {
    return (
      <Flex
        flex={1}
        justify="center"
        align="center"
        vertical
        gap="middle"
        style={{ padding: token.paddingXL }}
      >
        <Empty
          image={<DatabaseOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
          description={
            <Flex vertical gap="small" align="center">
              <Text type="secondary">No knowledge bases yet</Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                Create a knowledge base to start uploading documents
              </Text>
            </Flex>
          }
        />
        <Button type="primary" onClick={onCreate}>
          Create Knowledge Base
        </Button>
      </Flex>
    );
  }

  return (
    <Flex vertical gap="middle" style={{ padding: token.padding }}>
      <Flex justify="space-between" align="center">
        <Text strong style={{ fontSize: 16 }}>
          Knowledge Bases
        </Text>
        <Button type="primary" size="small" onClick={onCreate} aria-label="Create knowledge base">
          New
        </Button>
      </Flex>

      <Flex vertical gap="small">
        {knowledgeBases.map((kb) => (
          <Card
            key={kb.id}
            size="small"
            hoverable
            onClick={() => onSelect(kb)}
            style={{
              borderColor: selectedId === kb.id ? token.colorPrimary : token.colorBorderSecondary,
              background: selectedId === kb.id ? token.colorPrimaryBg : token.colorBgContainer,
              cursor: 'pointer',
            }}
            extra={
              <Popconfirm
                title="Delete knowledge base?"
                description="This will also delete all documents and chunks."
                onConfirm={(e) => {
                  e?.stopPropagation();
                  onDelete(kb.id);
                }}
                onCancel={(e) => e?.stopPropagation()}
                okText="Delete"
                cancelText="Cancel"
                okButtonProps={{ danger: true }}
              >
                <Button
                  type="text"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={(e) => e.stopPropagation()}
                  aria-label={`Delete knowledge base ${kb.name}`}
                />
              </Popconfirm>
            }
          >
            <Flex vertical gap="small">
              <Text strong style={{ fontSize: 14 }}>
                {kb.name}
              </Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                {kb.backend || 'default'} · {new Date(kb.created_at).toLocaleDateString()}
              </Text>
            </Flex>
          </Card>
        ))}
      </Flex>
    </Flex>
  );
};
