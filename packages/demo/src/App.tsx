import React, { useState, useEffect, useRef } from 'react';
import { Bubble, Conversations, Sender, Think, Welcome, XProvider, ThoughtChain } from '@ant-design/x';
import { XMarkdown } from '@ant-design/x-markdown';
import { theme, Button, Divider, Flex, Spin, Typography } from 'antd';
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons';
import { AgentClient, useAgentChat, Session, CopConMessage, Step, ToolCallPart, TodoList, TodoItemProps, Todo } from '@copcon/ui';
import './App.css';

const { useToken } = theme;
const { Text } = Typography;

const client = new AgentClient({ baseUrl: '' });

const MarkdownContent: React.FC<{ content: string }> = ({ content }) => (
  <XMarkdown content={content} />
);

const mapToolCallStatus = (state: string): 'loading' | 'success' | 'error' => {
  switch (state) {
    case 'pending': return 'loading';
    case 'running': return 'loading';
    case 'complete': return 'success';
    case 'error': return 'error';
    default: return 'loading';
  }
};

interface BubbleItem {
  key: string;
  role: string;
  content: string | React.ReactNode;
  loading?: boolean;
}

const StepContent: React.FC<{ step: Step }> = ({ step }) => {
  const toolCallParts: ToolCallPart[] = [];

  return (
    <>
      {step.parts.map((part, index) => {
        switch (part.type) {
          case 'text':
            return <MarkdownContent key={index} content={part.text} />;
          case 'reasoning':
            return (
              <Think key={index} title="Thinking" defaultExpanded>
                <MarkdownContent content={part.text} />
              </Think>
            );
          case 'tool-call':
            toolCallParts.push(part);
            return null; // Will render as ThoughtChain below
          default:
            return null;
        }
      })}
      {toolCallParts.length > 0 && (
        <ThoughtChain
          items={toolCallParts.map((part) => ({
            key: part.toolCallId,
            title: part.toolName,
            status: mapToolCallStatus(part.state),
            description: part.args,
            content: part.output
              ? <MarkdownContent content={part.output} />
              : undefined,
          }))}
        />
      )}
    </>
  );
};

const App: React.FC = () => {
  const { token } = useToken();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeKey, setActiveKey] = useState<string>('');
  const [loadingSessions, setLoadingSessions] = useState(true);
  const [todos, setTodos] = useState<TodoItemProps[]>([]);
  const [sidebarVisible, setSidebarVisible] = useState(true);

  const { 
    messages, 
    isRequesting, 
    sendMessage, 
    abort
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
      loadTodos(activeKey);
    } else {
      setTodos([]);
    }
  }, [activeKey]);

  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [messages]);

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

  const loadTodos = async (sessionId: string) => {
    try {
      const result = await client.getTodos(sessionId);
      const todoItems: TodoItemProps[] = (result.todos || []).map((todo: Todo) => ({
        id: todo.id,
        content: todo.content,
        status: todo.status,
        activeForm: todo.active_form,
        result: todo.result,
      }));
      setTodos(todoItems);
    } catch (error) {
      console.error('Failed to load todos:', error);
      setTodos([]);
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

  const renderMessageContent = (msg: CopConMessage) => {
    if (!msg.steps || msg.steps.length === 0) {
      return null;
    }

    return (
      <>
        {msg.steps.map((step, stepIndex) => (
          <React.Fragment key={stepIndex}>
            {stepIndex > 0 && <Divider style={{ margin: '8px 0' }} />}
            <StepContent step={step} />
          </React.Fragment>
        ))}
      </>
    );
  };

  const bubbleItems: BubbleItem[] = [];
  
  messages.forEach((msg: CopConMessage) => {
    const isLastAssistant = 
      msg.role === 'assistant' && 
      messages.indexOf(msg) === messages.length - 1;

    if (msg.role === 'user') {
      const userText = msg.steps[0]?.parts.find(p => p.type === 'text')?.text || '';
      bubbleItems.push({
        key: msg.id,
        role: 'user',
        content: userText,
      });
    } else {
      bubbleItems.push({
        key: msg.id,
        role: 'ai',
        content: renderMessageContent(msg),
        loading: isLastAssistant && isRequesting && 
          !msg.steps.some(s => s.parts.some(p => 
            (p.type === 'text' && p.text) || 
            (p.type === 'reasoning' && p.text) || 
            p.type === 'tool-call'
          )),
      });
    }
  });

  const roleConfig = {
    ai: {
      placement: 'start' as const,
      variant: 'filled' as const,
      shape: 'default' as const,
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
  };

  const handleSubmit = (text: string) => {
    if (text.trim()) {
      sendMessage(text);
      senderRef.current?.clear();
    }
  };

  const handleStatusChange = (id: string, status: string) => {
    setTodos((prev) =>
      prev.map((todo) => (todo.id === id ? { ...todo, status: status as TodoItemProps['status'] } : todo))
    );
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
                  loading={isRequesting}
                  onSubmit={handleSubmit}
                  onCancel={abort}
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

        {sidebarVisible && (
          <Flex
            vertical
            style={{
              width: 320,
              borderLeft: `1px solid ${token.colorBorderSecondary}`,
              background: token.colorBgContainer,
            }}
            className="todo-sidebar"
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
                Tasks
              </Text>
              <Button
                type="text"
                size="small"
                icon={<MenuFoldOutlined />}
                onClick={() => setSidebarVisible(false)}
              />
            </Flex>
            <Flex
              flex={1}
              vertical
              style={{
                padding: token.paddingSM,
                overflow: 'auto',
              }}
            >
              <TodoList todos={todos} onStatusChange={handleStatusChange} />
            </Flex>
          </Flex>
        )}

        {!sidebarVisible && (
          <Button
            type="text"
            icon={<MenuUnfoldOutlined />}
            onClick={() => setSidebarVisible(true)}
            style={{
              position: 'fixed',
              right: 16,
              top: 16,
              zIndex: 1000,
            }}
            className="sidebar-toggle-button"
          />
        )}
      </Flex>
    </XProvider>
  );
};

export default App;