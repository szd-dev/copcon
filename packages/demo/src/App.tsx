import React, { useState, useEffect, useRef } from 'react';
import { Bubble, Conversations, Sender, Think, Welcome, XProvider } from '@ant-design/x';
import { XMarkdown } from '@ant-design/x-markdown';
import { theme, Button, Flex, Spin, Typography } from 'antd';
import { AgentClient, useAgentChat, Session, Message, ToolExecution } from '@copcon/ui';

const { useToken } = theme;
const { Text } = Typography;

const client = new AgentClient({ baseUrl: '' });

const MarkdownContent: React.FC<{ content: string }> = ({ content }) => (
  <XMarkdown content={content} />
);

interface BubbleItem {
  key: string;
  role: string;
  content: string;
  header?: React.ReactNode;
  loading?: boolean;
}

const App: React.FC = () => {
  const { token } = useToken();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeKey, setActiveKey] = useState<string>('');
  const [loadingSessions, setLoadingSessions] = useState(true);

  const { 
    messages, 
    isLoading, 
    toolExecutions, 
    sendMessage, 
    stopGeneration, 
    loadMessages 
  } = useAgentChat({
    client,
    sessionId: activeKey,
  });

  const listRef = useRef<HTMLDivElement>(null);
  const senderRef = useRef<any>(null);

  useEffect(() => {
    loadSessions();
  }, []);

  useEffect(() => {
    if (activeKey) {
      loadMessages();
    }
  }, [activeKey]);

  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [messages, toolExecutions]);

  const loadSessions = async () => {
    setLoadingSessions(true);
    try {
      const result = await client.getSessions();
      setSessions(result.sessions || []);
      if (result.sessions?.length > 0 && !activeKey) {
        setActiveKey(result.sessions[0].id);
      }
    } catch (error) {
      console.error('Failed to load sessions:', error);
    } finally {
      setLoadingSessions(false);
    }
  };

  const handleCreateSession = async () => {
    try {
      const session = await client.createSession();
      setSessions([session, ...sessions]);
      setActiveKey(session.id);
    } catch (error) {
      console.error('Failed to create session:', error);
    }
  };

  const handleDeleteSession = async (key: string) => {
    try {
      await client.deleteSession(key);
      const remaining = sessions.filter(s => s.id !== key);
      setSessions(remaining);
      if (activeKey === key) {
        setActiveKey(remaining.length > 0 ? remaining[0].id : '');
      }
    } catch (error) {
      console.error('Failed to delete session:', error);
    }
  };

  const conversationItems = sessions.map((session) => ({
    key: session.id,
    label: session.title || 'New Chat',
  }));

  const bubbleItems: BubbleItem[] = [];
  
  messages.forEach((msg: Message) => {
    const isLastAssistant = 
      msg.role === 'assistant' && 
      messages.indexOf(msg) === messages.length - 1;
    
    bubbleItems.push({
      key: msg.id,
      role: msg.role === 'user' ? 'user' : msg.role === 'tool' ? 'tool' : 'ai',
      content: msg.content,
      loading: isLastAssistant && isLoading && !msg.content,
      header: msg.reasoning ? (
        <Think
          title="💭 Thinking..."
          loading={isLastAssistant && isLoading}
          defaultExpanded={true}
          styles={{
            root: {
              background: token.colorBgContainer,
              borderRadius: token.borderRadius,
              marginBottom: token.marginXS,
              border: `1px solid ${token.colorInfoBorder}`,
            },
            content: {
              color: token.colorTextSecondary,
              fontSize: 12,
              lineHeight: 1.5,
              whiteSpace: 'pre-wrap',
            },
          }}
        >
          {msg.reasoning}
        </Think>
      ) : undefined,
    });
  });

  toolExecutions.forEach((tool: ToolExecution) => {
    bubbleItems.push({
      key: tool.id,
      role: 'tool',
      content: tool.output 
        ? `Arguments:\n${JSON.stringify(tool.arguments, null, 2)}\n\nOutput:\n${tool.output}`
        : JSON.stringify(tool.arguments, null, 2),
      loading: tool.status === 'running',
      header: (
        <Text type="secondary" style={{ fontSize: 12 }}>
          🔧 {tool.name}
        </Text>
      ),
    });
  });

  const roleConfig = {
    ai: {
      placement: 'start' as const,
      variant: 'filled' as const,
      shape: 'default' as const,
      contentRender: (content: string) => <MarkdownContent content={content} />,
      styles: {
        content: {
          maxWidth: '100%',
        },
      },
    },
    user: {
      placement: 'end' as const,
      variant: 'filled' as const,
      shape: 'default' as const,
    },
    tool: {
      placement: 'start' as const,
      variant: 'outlined' as const,
      shape: 'default' as const,
      styles: {
        content: {
          fontFamily: 'monospace',
          fontSize: 12,
          whiteSpace: 'pre-wrap',
          maxHeight: 300,
          overflow: 'auto',
        },
      },
    },
  };

  const handleSubmit = (text: string) => {
    if (text.trim()) {
      sendMessage(text);
      senderRef.current?.clear();
    }
  };

  return (
    <XProvider>
      <Flex
        style={{
          height: '100vh',
          background: token.colorBgContainer,
        }}
      >
        <Flex
          vertical
          style={{
            width: 280,
            borderRight: `1px solid ${token.colorBorderSecondary}`,
            background: token.colorBgLayout,
          }}
        >
          <Flex
            justify="space-between"
            align="center"
            style={{
              padding: token.padding,
              borderBottom: `1px solid ${token.colorBorderSecondary}`,
            }}
          >
            <Text strong style={{ fontSize: 16 }}>
              CopCon Chat
            </Text>
            <Button type="primary" size="small" onClick={handleCreateSession}>
              New
            </Button>
          </Flex>
          <Flex
            flex={1}
            vertical
            style={{
              padding: token.paddingSM,
              overflow: 'hidden',
            }}
          >
            {loadingSessions ? (
              <Flex justify="center" align="center" flex={1}>
                <Spin />
              </Flex>
            ) : (
              <Conversations
                activeKey={activeKey}
                onActiveChange={(key) => setActiveKey(key)}
                items={conversationItems}
                menu={(conversation) => ({
                  items: [
                    {
                      key: 'delete',
                      label: 'Delete',
                      icon: <span>🗑️</span>,
                    },
                  ],
                  onClick: () => {
                    handleDeleteSession(conversation.key);
                  },
                })}
                style={{
                  height: '100%',
                }}
              />
            )}
          </Flex>
        </Flex>

        <Flex
          vertical
          flex={1}
          style={{
            background: token.colorBgLayout,
          }}
        >
          {activeKey ? (
            <>
              <Flex
                ref={listRef}
                flex={1}
                vertical
                gap="middle"
                style={{
                  padding: token.paddingLG,
                  overflow: 'auto',
                }}
              >
                {bubbleItems.length > 0 ? (
                  <Bubble.List
                    items={bubbleItems}
                    role={roleConfig}
                    autoScroll
                    styles={{
                      bubble: {
                        maxWidth: '80%',
                      },
                    }}
                  />
                ) : (
                  <Welcome
                    variant="borderless"
                    styles={{
                      root: {
                        padding: token.paddingXL,
                      },
                    }}
                  />
                )}
              </Flex>

              <Flex
                style={{
                  padding: token.paddingLG,
                  borderTop: `1px solid ${token.colorBorderSecondary}`,
                  background: token.colorBgContainer,
                }}
              >
                <Sender
                  ref={senderRef}
                  loading={isLoading}
                  onSubmit={handleSubmit}
                  onCancel={stopGeneration}
                  placeholder="Type a message..."
                  style={{
                    flex: 1,
                  }}
                  autoSize={{ minRows: 3, maxRows: 6 }}
                />
              </Flex>
            </>
          ) : (
            <Flex
              flex={1}
              justify="center"
              align="center"
              vertical
              gap="middle"
            >
              <Text type="secondary">No conversation selected</Text>
              <Button type="primary" onClick={handleCreateSession}>
                Start a new chat
              </Button>
            </Flex>
          )}
        </Flex>
      </Flex>
    </XProvider>
  );
};

export default App;