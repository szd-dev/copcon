import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  Tag,
  Empty,
  Spin,
  Button,
  Popconfirm,
  message,
  Typography,
  Flex,
  Modal,
  Form,
  Input,
  Select,
  theme,
  Descriptions,
} from 'antd';
import { PlusOutlined, DeleteOutlined, ApiOutlined } from '@ant-design/icons';
import { useClient } from '../context/ClientContext';
import type { MCPServerInfo, MCPServerConfig } from '@copcon/chat-core';

const { useToken } = theme;
const { Text, Title } = Typography;

const MCPPage: React.FC = () => {
  const { token } = useToken();
  const client = useClient();
  const [servers, setServers] = useState<MCPServerInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<MCPServerInfo | undefined>();
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [adding, setAdding] = useState(false);

  const loadServers = useCallback(async () => {
    setLoading(true);
    try {
      const result = await client.listMCPServers();
      setServers(result.servers || []);
    } catch {
      message.error('Failed to load MCP servers');
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    loadServers();
  }, [loadServers]);

  useEffect(() => {
    if (selected) {
      const updated = servers.find((s) => s.name === selected.name);
      setSelected(updated);
    }
  }, [servers, selected]);

  const handleAdd = async (values: {
    name: string;
    type: string;
    command?: string;
    argsText?: string;
    url?: string;
  }) => {
    setAdding(true);
    try {
      const config: MCPServerConfig = {
        name: values.name,
        type: values.type,
      };
      if (values.type === 'stdio') {
        config.command = values.command;
        if (values.argsText?.trim()) {
          config.args = values.argsText
            .split(/[\s,]+/)
            .filter(Boolean);
        }
      } else {
        config.url = values.url;
      }
      const server = await client.addMCPServer(config);
      setServers((prev) => [server, ...prev]);
      setSelected(server);
      setAddModalOpen(false);
      message.success('MCP server added');
    } catch {
      message.error('Failed to add MCP server');
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await client.removeMCPServer(name);
      setServers((prev) => prev.filter((s) => s.name !== name));
      if (selected?.name === name) {
        setSelected(undefined);
      }
      message.success('MCP server removed');
    } catch {
      message.error('Failed to remove MCP server');
    }
  };

  const handleToggle = async (server: MCPServerInfo) => {
    try {
      if (server.enabled) {
        await client.disableMCPServer(server.name);
      } else {
        await client.enableMCPServer(server.name);
      }
      await loadServers();
      message.success(server.enabled ? 'Server disabled' : 'Server enabled');
    } catch {
      message.error('Failed to toggle server');
    }
  };

  return (
    <Flex style={{ height: '100%' }}>
      <Flex
        vertical
        style={{
          width: 320,
          borderRight: `1px solid ${token.colorBorderSecondary}`,
          overflow: 'auto',
        }}
      >
        <Flex vertical gap="middle" style={{ padding: token.padding }}>
          <Flex justify="space-between" align="center">
            <Text strong style={{ fontSize: 16 }}>
              MCP Servers
            </Text>
            <Button
              type="primary"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => setAddModalOpen(true)}
            >
              Add Server
            </Button>
          </Flex>

          {loading ? (
            <Flex justify="center" style={{ padding: token.paddingXL }}>
              <Spin />
            </Flex>
          ) : servers.length === 0 ? (
            <Flex
              flex={1}
              justify="center"
              align="center"
              vertical
              gap="middle"
              style={{ padding: token.paddingXL }}
            >
              <Empty
                image={
                  <ApiOutlined
                    style={{ fontSize: 48, color: token.colorTextDisabled }}
                  />
                }
                description={
                  <Flex vertical gap="small" align="center">
                    <Text type="secondary">No MCP servers yet</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      Add a server to extend agent capabilities
                    </Text>
                  </Flex>
                }
              />
            </Flex>
          ) : (
            <Flex vertical gap="small">
              {servers.map((server) => (
                <Card
                  key={server.name}
                  size="small"
                  hoverable
                  onClick={() => setSelected(server)}
                  style={{
                    borderColor:
                      selected?.name === server.name
                        ? token.colorPrimary
                        : token.colorBorderSecondary,
                    background:
                      selected?.name === server.name
                        ? token.colorPrimaryBg
                        : token.colorBgContainer,
                    cursor: 'pointer',
                  }}
                >
                  <Flex justify="space-between" align="center">
                    <Flex vertical gap={4} style={{ minWidth: 0, flex: 1 }}>
                      <Text
                        strong
                        ellipsis
                        style={{ fontSize: 14 }}
                      >
                        {server.name}
                      </Text>
                      <Flex gap={4}>
                        <Tag color="blue" style={{ margin: 0 }}>
                          {server.type}
                        </Tag>
                        <Tag
                          color={server.enabled ? 'green' : 'default'}
                          style={{ margin: 0 }}
                        >
                          {server.enabled ? 'enabled' : 'disabled'}
                        </Tag>
                      </Flex>
                    </Flex>
                    <Popconfirm
                      title="Delete this server?"
                      description={`Remove "${server.name}" from MCP configuration.`}
                      onConfirm={(e) => {
                        e?.stopPropagation();
                        handleDelete(server.name);
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
                      />
                    </Popconfirm>
                  </Flex>
                </Card>
              ))}
            </Flex>
          )}
        </Flex>
      </Flex>

      <Flex flex={1} style={{ overflow: 'auto' }}>
        {selected ? (
          <Flex
            vertical
            gap="middle"
            style={{ padding: token.paddingLG, width: '100%' }}
          >
            <Flex justify="space-between" align="center">
              <Title level={4} style={{ margin: 0 }}>
                {selected.name}
              </Title>
              <Flex gap="small">
                <Popconfirm
                  title={selected.enabled ? 'Disable this server?' : 'Enable this server?'}
                  onConfirm={() => handleToggle(selected)}
                  okText={selected.enabled ? 'Disable' : 'Enable'}
                  cancelText="Cancel"
                >
                  <Button>
                    {selected.enabled ? 'Disable' : 'Enable'}
                  </Button>
                </Popconfirm>
                <Popconfirm
                  title="Delete this server?"
                  description={`Remove "${selected.name}" from MCP configuration.`}
                  onConfirm={() => handleDelete(selected.name)}
                  okText="Delete"
                  cancelText="Cancel"
                  okButtonProps={{ danger: true }}
                >
                  <Button danger icon={<DeleteOutlined />}>
                    Delete
                  </Button>
                </Popconfirm>
              </Flex>
            </Flex>

            <Card size="small">
              <Descriptions column={1} size="small">
                <Descriptions.Item label="Name">
                  {selected.name}
                </Descriptions.Item>
                <Descriptions.Item label="Type">
                  <Tag color="blue">{selected.type}</Tag>
                </Descriptions.Item>
                <Descriptions.Item label="Enabled">
                  <Tag color={selected.enabled ? 'green' : 'default'}>
                    {selected.enabled ? 'Yes' : 'No'}
                  </Tag>
                </Descriptions.Item>
                {selected.command && (
                  <Descriptions.Item label="Command">
                    <Text code>{selected.command}</Text>
                  </Descriptions.Item>
                )}
                {selected.args && selected.args.length > 0 && (
                  <Descriptions.Item label="Args">
                    <Flex gap={4} wrap>
                      {selected.args.map((arg, i) => (
                        <Tag key={i}>{arg}</Tag>
                      ))}
                    </Flex>
                  </Descriptions.Item>
                )}
                {selected.url && (
                  <Descriptions.Item label="URL">
                    <Text code>{selected.url}</Text>
                  </Descriptions.Item>
                )}
                {selected.allowed_tools && (
                  <Descriptions.Item label="Allowed Tools">
                    <Flex gap={4} wrap vertical>
                      {selected.allowed_tools.include && (
                        <Flex gap={4} wrap>
                          <Text type="secondary">Include:</Text>
                          {selected.allowed_tools.include.map((t) => (
                            <Tag key={t} color="green">{t}</Tag>
                          ))}
                        </Flex>
                      )}
                      {selected.allowed_tools.exclude && (
                        <Flex gap={4} wrap>
                          <Text type="secondary">Exclude:</Text>
                          {selected.allowed_tools.exclude.map((t) => (
                            <Tag key={t} color="red">{t}</Tag>
                          ))}
                        </Flex>
                      )}
                    </Flex>
                  </Descriptions.Item>
                )}
                {selected.tools && selected.tools.length > 0 && (
                  <Descriptions.Item label="Tools">
                    <Flex gap={4} wrap>
                      {selected.tools.map((t) => (
                        <Tag key={t}>{t}</Tag>
                      ))}
                    </Flex>
                  </Descriptions.Item>
                )}
              </Descriptions>
            </Card>
          </Flex>
        ) : (
          <Flex
            flex={1}
            justify="center"
            align="center"
            style={{ padding: token.paddingXL }}
          >
            <Empty description="Select a server to view details" />
          </Flex>
        )}
      </Flex>

      <AddServerModal
        open={addModalOpen}
        onCancel={() => setAddModalOpen(false)}
        onSubmit={handleAdd}
        loading={adding}
      />
    </Flex>
  );
};

interface AddServerModalProps {
  open: boolean;
  onCancel: () => void;
  onSubmit: (values: {
    name: string;
    type: string;
    command?: string;
    argsText?: string;
    url?: string;
  }) => void;
  loading?: boolean;
}

const AddServerModal: React.FC<AddServerModalProps> = ({
  open,
  onCancel,
  onSubmit,
  loading,
}) => {
  const [form] = Form.useForm();
  const serverType = Form.useWatch('type', form);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      onSubmit(values);
      form.resetFields();
    } catch {
      return;
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  return (
    <Modal
      title="Add MCP Server"
      open={open}
      onCancel={handleCancel}
      footer={[
        <Button key="cancel" onClick={handleCancel}>
          Cancel
        </Button>,
        <Button key="submit" type="primary" loading={loading} onClick={handleSubmit}>
          Add
        </Button>,
      ]}
      destroyOnClose
    >
      <Form form={form} layout="vertical" autoComplete="off" initialValues={{ type: 'stdio' }}>
        <Form.Item
          label="Name"
          name="name"
          rules={[
            { required: true, message: 'Please enter a server name' },
            { min: 1, message: 'Name cannot be empty' },
          ]}
        >
          <Input placeholder="e.g. filesystem" autoFocus />
        </Form.Item>
        <Form.Item
          label="Type"
          name="type"
          rules={[{ required: true, message: 'Please select a type' }]}
        >
          <Select
            options={[
              { label: 'stdio', value: 'stdio' },
              { label: 'sse', value: 'sse' },
              { label: 'streamable-http', value: 'streamable-http' },
            ]}
          />
        </Form.Item>
        {serverType === 'stdio' && (
          <>
            <Form.Item
              label="Command"
              name="command"
              rules={[{ required: true, message: 'Please enter a command' }]}
            >
              <Input placeholder="e.g. npx" />
            </Form.Item>
            <Form.Item label="Args" name="argsText">
              <Input placeholder="Space or comma separated, e.g. -y, @modelcontextprotocol/server-filesystem" />
            </Form.Item>
          </>
        )}
        {(serverType === 'sse' || serverType === 'streamable-http') && (
          <Form.Item
            label="URL"
            name="url"
            rules={[{ required: true, message: 'Please enter a URL' }]}
          >
            <Input placeholder="e.g. http://localhost:3000/sse" />
          </Form.Item>
        )}
      </Form>
    </Modal>
  );
};

export default MCPPage;
