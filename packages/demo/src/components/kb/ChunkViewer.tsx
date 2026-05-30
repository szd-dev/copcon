import React, { useState, useEffect, useCallback } from 'react';
import { Drawer, Input, Flex, List, Typography, theme, Skeleton, Empty, Badge } from 'antd';
import { SearchOutlined, FileTextOutlined } from '@ant-design/icons';
import { XMarkdown } from '@ant-design/x-markdown';
import { useClient } from '../../context/ClientContext';
import type { Chunk } from '@copcon/chat-core';

const { useToken } = theme;
const { Text } = Typography;

interface ChunkViewerProps {
  open: boolean;
  kbId: string;
  docId: string;
  onClose: () => void;
}

const HighlightText: React.FC<{ text: string; keyword: string }> = ({ text, keyword }) => {
  const { token } = useToken();
  if (!keyword.trim()) {
    return <span>{text}</span>;
  }

  const parts = text.split(new RegExp(`(${keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));
  return (
    <span>
      {parts.map((part, i) =>
        part.toLowerCase() === keyword.toLowerCase() ? (
          <mark
            key={i}
            style={{
              background: token.colorWarningBg,
              color: token.colorWarningText,
              padding: '0 2px',
              borderRadius: token.borderRadiusSM,
            }}
          >
            {part}
          </mark>
        ) : (
          <span key={i}>{part}</span>
        )
      )}
    </span>
  );
};

export const ChunkViewer: React.FC<ChunkViewerProps> = ({ open, kbId, docId, onClose }) => {
  const { token } = useToken();
  const client = useClient();
  const [chunks, setChunks] = useState<Chunk[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');

  const loadChunks = useCallback(async () => {
    if (!kbId || !docId) return;
    setLoading(true);
    try {
      const result = await client.getDocumentChunks(kbId, docId);
      setChunks(result.chunks || []);
    } catch {
      setChunks([]);
    } finally {
      setLoading(false);
    }
  }, [client, kbId, docId]);

  useEffect(() => {
    if (open) {
      loadChunks();
      setSearchText('');
    }
  }, [open, loadChunks]);

  const filteredChunks = searchText.trim()
    ? chunks.filter(
        (c) =>
          c.content.toLowerCase().includes(searchText.toLowerCase()) ||
          c.context.toLowerCase().includes(searchText.toLowerCase())
      )
    : chunks;

  return (
    <Drawer title="Document Chunks" open={open} onClose={onClose} width={720}>
      <Flex vertical gap="middle" style={{ height: '100%' }}>
        <Input
          placeholder="Search within chunks..."
          value={searchText}
          onChange={(e) => setSearchText(e.target.value)}
          prefix={<SearchOutlined />}
          allowClear
          aria-label="Search chunks"
        />

        <Text type="secondary" style={{ fontSize: 12 }}>
          {filteredChunks.length} of {chunks.length} chunks
          {searchText.trim() ? ` matching "${searchText}"` : ''}
        </Text>

        {loading ? (
          <Skeleton active paragraph={{ rows: 8 }} />
        ) : filteredChunks.length > 0 ? (
          <List
            dataSource={filteredChunks}
            renderItem={(chunk) => (
              <List.Item>
                <Flex vertical gap="small" style={{ width: '100%' }}>
                  <Flex justify="space-between" align="center">
                    <Badge count={`#${chunk.index + 1}`} style={{ backgroundColor: token.colorPrimary }} />
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {chunk.token_count} tokens
                    </Text>
                  </Flex>

                  {chunk.context && (
                    <div
                      style={{
                        padding: token.paddingSM,
                        background: token.colorBgContainerDisabled,
                        borderRadius: token.borderRadiusSM,
                        fontSize: 12,
                        color: token.colorTextSecondary,
                      }}
                    >
                      <HighlightText text={chunk.context} keyword={searchText} />
                    </div>
                  )}

                  <div
                    style={{
                      padding: token.paddingSM,
                      background: token.colorBgContainer,
                      border: `1px solid ${token.colorBorderSecondary}`,
                      borderRadius: token.borderRadiusSM,
                    }}
                  >
                    <XMarkdown content={chunk.content} />
                  </div>
                </Flex>
              </List.Item>
            )}
          />
        ) : (
          <Empty
            image={<FileTextOutlined style={{ fontSize: 48, color: token.colorTextDisabled }} />}
            description={
              <Text type="secondary">
                {searchText.trim() ? 'No chunks match your search' : 'No chunks found'}
              </Text>
            }
          />
        )}
      </Flex>
    </Drawer>
  );
};
