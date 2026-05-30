import React, { useState, useRef } from 'react';
import { Drawer, Upload, Flex, Typography, theme, Progress, Alert, List, Button } from 'antd';
import { InboxOutlined, FileOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { useClient } from '../../context/ClientContext';

const { useToken } = theme;
const { Text } = Typography;
const { Dragger } = Upload;

const ALLOWED_TYPES = ['.pdf', '.md', '.txt', '.html'];
const MAX_SIZE_MB = 10;
const MAX_SIZE_BYTES = MAX_SIZE_MB * 1024 * 1024;

interface FileItem {
  id: string;
  file: File;
  status: 'pending' | 'uploading' | 'success' | 'error';
  progress: number;
  error?: string;
}

interface KBUploadProps {
  open: boolean;
  kbId: string;
  onClose: () => void;
  onSuccess: () => void;
}

export const KBUpload: React.FC<KBUploadProps> = ({ open, kbId, onClose, onSuccess }) => {
  const { token } = useToken();
  const client = useClient();
  const [items, setItems] = useState<FileItem[]>([]);
  const [uploading, setUploading] = useState(false);
  const draggerKey = useRef(0);

  const validateFile = (file: File): string | null => {
    const ext = '.' + file.name.split('.').pop()?.toLowerCase();
    if (!ALLOWED_TYPES.includes(ext)) {
      return `File type not allowed. Allowed: ${ALLOWED_TYPES.join(', ')}`;
    }
    if (file.size > MAX_SIZE_BYTES) {
      return `File too large. Max size: ${MAX_SIZE_MB}MB`;
    }
    return null;
  };

  const handleChange = (info: { file: { originFileObj?: File; name: string; size?: number; uid: string } }) => {
    const rawFile = info.file.originFileObj;
    if (!rawFile) return;

    const error = validateFile(rawFile);
    if (error) {
      setItems((prev) => [
        ...prev,
        {
          id: info.file.uid,
          file: rawFile,
          status: 'error',
          progress: 0,
          error,
        },
      ]);
      return;
    }

    setItems((prev) => {
      if (prev.some((p) => p.file.name === rawFile.name)) return prev;
      return [
        ...prev,
        {
          id: info.file.uid,
          file: rawFile,
          status: 'pending',
          progress: 0,
        },
      ];
    });
  };

  const updateItem = (id: string, updates: Partial<FileItem>) => {
    setItems((prev) => prev.map((item) => (item.id === id ? { ...item, ...updates } : item)));
  };

  const handleUploadAll = async () => {
    const pendingItems = items.filter((item) => item.status === 'pending');
    if (pendingItems.length === 0) return;

    setUploading(true);
    let hasError = false;

    for (const item of pendingItems) {
      updateItem(item.id, { status: 'uploading', progress: 50 });
      try {
        await client.uploadDocument(kbId, item.file);
        updateItem(item.id, { status: 'success', progress: 100 });
      } catch {
        updateItem(item.id, { status: 'error', progress: 0, error: 'Upload failed' });
        hasError = true;
      }
    }

    setUploading(false);
    if (!hasError) {
      onSuccess();
    }
  };

  const handleRemove = (id: string) => {
    setItems((prev) => prev.filter((item) => item.id !== id));
  };

  const handleClose = () => {
    setItems([]);
    draggerKey.current += 1;
    onClose();
  };

  const pendingCount = items.filter((i) => i.status === 'pending').length;
  const successCount = items.filter((i) => i.status === 'success').length;
  const errorCount = items.filter((i) => i.status === 'error').length;

  return (
    <Drawer title="Upload Documents" open={open} onClose={handleClose} width={560}>
      <Flex vertical gap="middle">
        <Alert
          message={`Supported formats: ${ALLOWED_TYPES.join(', ')} · Max size: ${MAX_SIZE_MB}MB per file`}
          type="info"
          showIcon
        />

        <Dragger
          key={draggerKey.current}
          multiple
          beforeUpload={() => false}
          onChange={handleChange}
          showUploadList={false}
          accept={ALLOWED_TYPES.join(',')}
          style={{
            background: token.colorBgContainerDisabled,
            border: `2px dashed ${token.colorBorder}`,
            borderRadius: token.borderRadiusLG,
          }}
        >
          <p className="ant-upload-drag-icon">
            <InboxOutlined style={{ color: token.colorPrimary, fontSize: 48 }} />
          </p>
          <Text strong>Click or drag files to upload</Text>
          <Text type="secondary" style={{ display: 'block', marginTop: token.marginXS }}>
            Multiple files supported
          </Text>
        </Dragger>

        {items.length > 0 && (
          <Flex vertical gap="small">
            <Flex justify="space-between" align="center">
              <Text strong>Files ({items.length})</Text>
              {pendingCount > 0 && (
                <Button type="primary" loading={uploading} onClick={handleUploadAll} size="small">
                  Upload {pendingCount} file{pendingCount > 1 ? 's' : ''}
                </Button>
              )}
            </Flex>

            <List
              size="small"
              dataSource={items}
              renderItem={(item) => (
                <List.Item
                  actions={[
                    item.status !== 'uploading' && (
                      <Button
                        key="remove"
                        type="text"
                        size="small"
                        danger
                        onClick={() => handleRemove(item.id)}
                        aria-label={`Remove ${item.file.name}`}
                      >
                        Remove
                      </Button>
                    ),
                  ]}
                >
                  <List.Item.Meta
                    avatar={
                      item.status === 'success' ? (
                        <CheckCircleOutlined style={{ color: token.colorSuccess, fontSize: 20 }} />
                      ) : item.status === 'error' ? (
                        <CloseCircleOutlined style={{ color: token.colorError, fontSize: 20 }} />
                      ) : (
                        <FileOutlined style={{ fontSize: 20, color: token.colorTextSecondary }} />
                      )
                    }
                    title={item.file.name}
                    description={
                      <Flex vertical gap="small">
                        {item.status === 'uploading' && (
                          <Progress percent={item.progress} size="small" status="active" />
                        )}
                        {item.error && (
                          <Text type="danger" style={{ fontSize: 12 }}>
                            {item.error}
                          </Text>
                        )}
                      </Flex>
                    }
                  />
                </List.Item>
              )}
            />

            {successCount > 0 && errorCount === 0 && !uploading && (
              <Alert message="All files uploaded successfully" type="success" showIcon />
            )}
            {errorCount > 0 && !uploading && (
              <Alert
                message={`${errorCount} file${errorCount > 1 ? 's' : ''} failed to upload`}
                type="error"
                showIcon
              />
            )}
          </Flex>
        )}
      </Flex>
    </Drawer>
  );
};
