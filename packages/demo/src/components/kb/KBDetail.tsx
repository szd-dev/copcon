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
  Modal,
  Tooltip,
  message,
  Spin,
} from 'antd';
import {
  FileTextOutlined,
  UploadOutlined,
  EyeOutlined,
  DeleteOutlined,
  ExperimentOutlined,
} from '@ant-design/icons';
import type { KnowledgeBase, Document, DocumentStatus } from '@copcon/chat-core';
import { useClient } from '../../context/ClientContext';

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
  indexing: 'processing',
  ready: 'success',
  error: 'error',
};

const STATUS_LABELS: Record<DocumentStatus, string> = {
  pending: 'Pending',
  parsing: 'Parsing',
  indexing: 'Indexing',
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
  const client = useClient();
  const [statusFilter, setStatusFilter] = useState<DocumentStatus | 'all'>('all');
  const [sortOrder, setSortOrder] = useState<'newest' | 'oldest' | 'name'>('newest');
  const [contentModal, setContentModal] = useState<{ docId: string; filename: string } | null>(null);
  const [content, setContent] = useState<string>('');
  const [contentLoading, setContentLoading] = useState(false);

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

  const pendingCount = documents.filter((d) => d.status === 'pending').length;
  const parsingCount = documents.filter((d) => d.status === 'parsing').length;
  const indexingCount = documents.filter((d) => d.status === 'indexing').length;
  const readyCount = documents.filter((d) => d.status === 'ready').length;
  const errorCount = documents.filter((d) => d.status === 'error').length;

  const handleViewContent = async (docId: string, filename: string) => {
    if (!knowledgeBase) return;
    setContentModal({ docId, filename });
    setContentLoading(true);
    try {
      const doc = await client.getDocumentContent(knowledgeBase.id, docId);
      setContent(doc.content || '');
    } catch (error) {
      message.error('Failed to load content');
      setContent('Failed to load content');
    } finally {
      setContentLoading(false);
    }
  };

  const columns = [
    {
      title: 'Filename',
      dataIndex: 'filename',
      key: 'filename',
      render: (text: string, record: Document) => (
        <Typography.Link onClick={() => handleViewContent(record.id, record.filename)}>
          {text}
        </Typography.Link>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: DocumentStatus, record: Document) => {
        const badge = <Tag color={STATUS_COLORS[status] || 'default'}>{STATUS_LABELS[status] || status}</Tag>;
        if (status === 'error' && record.error_msg) {
          return <Tooltip title={record.error_msg}>{badge}</Tooltip>;
        }
        return badge;
      },
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

        <Flex gap="middle" style={{ marginBottom: 16 }}>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Documents" value={documents.length} />
            </Card>
          </Flex>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Pending" value={pendingCount} valueStyle={{ color: token.colorTextQuaternary }} />
            </Card>
          </Flex>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Parsing" value={parsingCount} valueStyle={{ color: token.colorPrimary }} />
            </Card>
          </Flex>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Indexing" value={indexingCount} valueStyle={{ color: token.colorPrimary }} />
            </Card>
          </Flex>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Ready" value={readyCount} valueStyle={{ color: token.colorSuccess }} />
            </Card>
          </Flex>
          <Flex flex={1}>
            <Card size="small" style={{ width: '100%' }}>
              <Statistic title="Errors" value={errorCount} valueStyle={{ color: token.colorError }} />
            </Card>
          </Flex>
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
              { value: 'indexing', label: 'Indexing' },
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

      <Modal
        title={`Content: ${contentModal?.filename}`}
        open={!!contentModal}
        onCancel={() => setContentModal(null)}
        footer={null}
        width={800}
      >
        {contentLoading ? (
          <Spin />
        ) : (
          <Typography.Paragraph style={{ maxHeight: 500, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
            {content || '(No content)'}
          </Typography.Paragraph>
        )}
      </Modal>
    </Flex>
  );
};
