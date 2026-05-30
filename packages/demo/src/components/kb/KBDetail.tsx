import React from 'react';
import { useState } from 'react';
import {
  Table,
  Flex,
  Empty,
  Typography,
  theme,
  Tag,
  Button,
  Space,
  Skeleton,
  Select,
  Statistic,
  Card,
} from 'antd';
import {
  FileTextOutlined,
  UploadOutlined,
  EyeOutlined,
  DeleteOutlined,
  ExperimentOutlined,
} from '@ant-design/icons';
import type { KnowledgeBase, Document, DocumentStatus } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

interface KBDetailProps {
  knowledgeBase?: KnowledgeBase;
  documents: Document[];
  loading: boolean;
  onUpload: () => void;
  onViewChunks: (docId: string) => void;
  onDeleteDocument: (docId: string) => void;
  onTestRetrieval: () => void;
}

const STATUS_COLORS: Record<DocumentStatus, string> = {
  pending: 'default',
  parsing: 'processing',
  ready: 'success',
  error: 'error',
};

const STATUS_LABELS: Record<DocumentStatus, string> = {
  pending: 'Pending',
  parsing: 'Parsing',
  ready: 'Ready',
  error: 'Error',
};

export const KBDetail: React.FC<KBDetailProps> = ({
  knowledgeBase,
  documents,
  loading,
  onUpload,
  onViewChunks,
  onDeleteDocument,
  onTestRetrieval,
}) => {
  const { token } = useToken();
  const [statusFilter, setStatusFilter] = useState<DocumentStatus | 'all'>('all');
  const [sortOrder, setSortOrder] = useState<'newest' | 'oldest' | 'name'>('newest');

  if (!knowledgeBase) {
    return (
      <Flex
        flex={1}
        justify="center"
        align="center"
        style={{ height: '100%', background: token.colorBgLayout }}
      >
        <Empty
          image={<FileTextOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
          description={<Text type="secondary">Select a knowledge base to view details</Text>}
        />
      </Flex>
    );
  }

  const filteredDocs = documents.filter((doc) =>
    statusFilter === 'all' ? true : doc.status === statusFilter
  );

  const sortedDocs = [...filteredDocs].sort((a, b) => {
    switch (sortOrder) {
      case 'newest':
        return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
      case 'oldest':
        return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
      case 'name':
        return a.filename.localeCompare(b.filename);
      default:
        return 0;
    }
  });

  const readyCount = documents.filter((d) => d.status === 'ready').length;
  const pendingCount = documents.filter((d) => d.status === 'pending' || d.status === 'parsing').length;
  const errorCount = documents.filter((d) => d.status === 'error').length;
  const totalChunks = documents.reduce((sum, d) => sum + d.chunk_count, 0);
  const totalTokens = documents.reduce((sum, d) => sum + d.token_count, 0);

  const columns = [
    {
      title: 'Filename',
      dataIndex: 'filename',
      key: 'filename',
      render: (text: string) => <Text strong>{text}</Text>,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: DocumentStatus) => (
        <Tag color={STATUS_COLORS[status] || 'default'}>{STATUS_LABELS[status] || status}</Tag>
      ),
    },
    {
      title: 'Chunks',
      dataIndex: 'chunk_count',
      key: 'chunk_count',
      width: 100,
    },
    {
      title: 'Tokens',
      dataIndex: 'token_count',
      key: 'token_count',
      width: 100,
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleDateString(),
      width: 120,
    },
    {
      title: 'Actions',
      key: 'actions',
      width: 160,
      render: (_: unknown, record: Document) => (
        <Space size="small">
          <Button
            type="text"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => onViewChunks(record.id)}
            aria-label={`View chunks for ${record.filename}`}
          />
          <Button
            type="text"
            size="small"
            danger
            icon={<DeleteOutlined />}
            onClick={() => onDeleteDocument(record.id)}
            aria-label={`Delete document ${record.filename}`}
          />
        </Space>
      ),
    },
  ];

  return (
    <Flex vertical style={{ height: '100%', overflow: 'auto' }}>
      <Flex
        vertical
        gap="middle"
        style={{
          padding: token.padding,
          borderBottom: `1px solid ${token.colorBorderSecondary}`,
          background: token.colorBgContainer,
        }}
      >
        <Flex justify="space-between" align="center">
          <Flex vertical>
            <Text strong style={{ fontSize: 18 }}>
              {knowledgeBase.name}
            </Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {knowledgeBase.backend || 'default'} · Created {new Date(knowledgeBase.created_at).toLocaleDateString()}
            </Text>
          </Flex>
          <Space>
            <Button icon={<UploadOutlined />} onClick={onUpload} aria-label="Upload documents">
              Upload
            </Button>
            <Button icon={<ExperimentOutlined />} onClick={onTestRetrieval} aria-label="Test retrieval">
              Test
            </Button>
          </Space>
        </Flex>

        <Flex gap="large">
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Documents" value={documents.length} />
          </Card>
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Ready" value={readyCount} valueStyle={{ color: token.colorSuccess }} />
          </Card>
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Pending" value={pendingCount} valueStyle={{ color: token.colorWarning }} />
          </Card>
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Errors" value={errorCount} valueStyle={{ color: token.colorError }} />
          </Card>
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Total Chunks" value={totalChunks} />
          </Card>
          <Card size="small" style={{ flex: 1 }}>
            <Statistic title="Total Tokens" value={totalTokens} />
          </Card>
        </Flex>

        <Flex gap="middle" align="center">
          <Select
            value={statusFilter}
            onChange={setStatusFilter}
            style={{ width: 140 }}
            options={[
              { value: 'all', label: 'All Statuses' },
              { value: 'pending', label: 'Pending' },
              { value: 'parsing', label: 'Parsing' },
              { value: 'ready', label: 'Ready' },
              { value: 'error', label: 'Error' },
            ]}
            aria-label="Filter by status"
          />
          <Select
            value={sortOrder}
            onChange={setSortOrder}
            style={{ width: 140 }}
            options={[
              { value: 'newest', label: 'Newest first' },
              { value: 'oldest', label: 'Oldest first' },
              { value: 'name', label: 'Name' },
            ]}
            aria-label="Sort documents"
          />
        </Flex>
      </Flex>

      <Flex flex={1} vertical style={{ padding: token.padding }}>
        {loading ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : sortedDocs.length > 0 ? (
          <Table
            dataSource={sortedDocs}
            columns={columns}
            rowKey="id"
            size="small"
            pagination={{ pageSize: 20, hideOnSinglePage: true }}
          />
        ) : (
          <Empty
            image={<FileTextOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
            description={
              <Flex vertical gap="small" align="center">
                <Text type="secondary">No documents yet</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  Upload documents to this knowledge base
                </Text>
                <Button type="primary" icon={<UploadOutlined />} onClick={onUpload}>
                  Upload Documents
                </Button>
              </Flex>
            }
          />
        )}
      </Flex>
    </Flex>
  );
};
