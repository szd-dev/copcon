import React, { useState } from 'react';
import {
  Modal,
  Input,
  Select,
  Button,
  Flex,
  List,
  Typography,
  theme,
  Empty,
  Card,
  Tag,
  Skeleton,
} from 'antd';
import { SearchOutlined, ExperimentOutlined } from '@ant-design/icons';
import { XMarkdown } from '@ant-design/x-markdown';
import { useClient } from '../../context/ClientContext';
import type { Chunk } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

interface KBRetrievalTestProps {
  open: boolean;
  kbId: string;
  onClose: () => void;
}

const getScoreColor = (score: number, token: typeof theme.useToken extends () => { token: infer T } ? T : never): string => {
  if (score >= 0.8) return token.colorSuccess;
  if (score >= 0.6) return token.colorWarning;
  return token.colorError;
};

const getScoreLabel = (score: number): string => {
  if (score >= 0.8) return 'High';
  if (score >= 0.6) return 'Medium';
  return 'Low';
};

export const KBRetrievalTest: React.FC<KBRetrievalTestProps> = ({ open, kbId, onClose }) => {
  const { token } = useToken();
  const client = useClient();
  const [query, setQuery] = useState('');
  const [topK, setTopK] = useState(5);
  const [results, setResults] = useState<Chunk[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const handleSearch = async () => {
    if (!query.trim()) return;
    setLoading(true);
    setSearched(true);
    try {
      const result = await client.testRetrieval(kbId, query.trim(), topK);
      setResults(result.results || []);
    } catch {
      setResults([]);
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setQuery('');
    setResults([]);
    setSearched(false);
    onClose();
  };

  return (
    <Modal
      title={
        <Flex gap="small" align="center">
          <ExperimentOutlined />
          <span>Test Retrieval</span>
        </Flex>
      }
      open={open}
      onCancel={handleClose}
      footer={null}
      width={720}
    >
      <Flex vertical gap="middle">
        <Flex gap="middle">
          <Input
            placeholder="Enter your query..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onPressEnter={handleSearch}
            prefix={<SearchOutlined />}
            style={{ flex: 1 }}
            aria-label="Retrieval query"
          />
          <Select
            value={topK}
            onChange={setTopK}
            style={{ width: 100 }}
            options={[
              { value: 3, label: 'Top 3' },
              { value: 5, label: 'Top 5' },
              { value: 10, label: 'Top 10' },
              { value: 20, label: 'Top 20' },
            ]}
            aria-label="Number of results"
          />
          <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch} loading={loading}>
            Search
          </Button>
        </Flex>

        {loading ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : searched && results.length === 0 ? (
          <Empty
            image={<ExperimentOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
            description={
              <Flex vertical gap="small" align="center">
                <Text type="secondary">No results found</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  Try a different query or check if documents are indexed
                </Text>
              </Flex>
            }
          />
        ) : results.length > 0 ? (
          <Flex vertical gap="small">
            <Text type="secondary" style={{ fontSize: 12 }}>
              {results.length} result{results.length > 1 ? 's' : ''} for &quot;{query}&quot;
            </Text>
            <List
              dataSource={results}
              renderItem={(chunk, index) => (
                <List.Item>
                  <Card
                    size="small"
                    style={{ width: '100%' }}
                    title={
                      <Flex justify="space-between" align="center">
                        <Text strong>Result #{index + 1}</Text>
                        <Tag
                          color={
                            chunk.score >= 0.8 ? 'success' : chunk.score >= 0.6 ? 'warning' : 'error'
                          }
                        >
                          Score: {chunk.score.toFixed(3)} ({getScoreLabel(chunk.score)})
                        </Tag>
                      </Flex>
                    }
                  >
                    <Flex vertical gap="small">
                      <div
                        style={{
                          height: 4,
                          width: '100%',
                          background: token.colorBorderSecondary,
                          borderRadius: token.borderRadiusSM,
                        }}
                      >
                        <div
                          style={{
                            height: '100%',
                            width: `${Math.min(chunk.score * 100, 100)}%`,
                            background: getScoreColor(chunk.score, token),
                            borderRadius: token.borderRadiusSM,
                            transition: 'width 0.3s ease',
                          }}
                        />
                      </div>
                      <XMarkdown content={chunk.content} />
                      {chunk.context && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          Context: {chunk.context}
                        </Text>
                      )}
                    </Flex>
                  </Card>
                </List.Item>
              )}
            />
          </Flex>
        ) : (
          <Empty
            image={<ExperimentOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
            description={
              <Flex vertical gap="small" align="center">
                <Text type="secondary">Enter a query to test retrieval</Text>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  This searches the knowledge base using vector similarity
                </Text>
              </Flex>
            }
          />
        )}
      </Flex>
    </Modal>
  );
};
