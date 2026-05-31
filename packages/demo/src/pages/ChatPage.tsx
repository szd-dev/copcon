import React, { useState, useEffect, useRef } from 'react';
import { Bubble, Conversations, Sender, Welcome } from '@ant-design/x';
import { theme, Button, Divider, Flex, Spin, Typography, message, Segmented } from 'antd';
import { MenuFoldOutlined, MenuUnfoldOutlined, CheckSquareOutlined, DatabaseOutlined } from '@ant-design/icons';
import type { Session, CopConMessage, Step, ToolCallPart, Todo } from '@copcon/chat-core';
import { useChat } from '@copcon/chat-react';
import { useClient } from '../context/ClientContext';
import { StreamMarkdown } from '../components/StreamMarkdown';
import { ThinkingBlock } from '../components/ThinkingBlock';
import { ToolCallCard } from '../components/ToolCallCard';
import { HumanInteraction } from '../components/HumanInteraction';
import { TodoList } from '../components/TodoList';
import { SubagentCard } from '../components/SubagentCard';
import { MemoryPanel } from '../components/memory/MemoryPanel';

const { useToken } = theme;
const { Text } = Typography;

interface BubbleItem {
  key: string;
  role: string;
  content: string | React.ReactNode;
  loading?: boolean;
}

const StepContent: React.FC<{ step: Step; sessionId: string }> = ({
  step,
  sessionId,
}) => {
  const client = useClient();
  return (
    <>
      {step.parts.map((part, index) => {
        switch (part.type) {
          case 'text':
            return <StreamMarkdown key={index} content={part.text} />;
          case 'reasoning':
            return <ThinkingBlock key={index} part={part} />;
          case 'tool-call': {
            const toolPart = part as ToolCallPart;
            if (toolPart.state === 'waiting_for_input' && toolPart.interrupt) {
              return <HumanInteraction key={index} part={toolPart} sessionId={sessionId} client={client} />;
            }
            if (toolPart.toolName === 'delegate_to_agent' && toolPart.args) {
              const args = typeof toolPart.args === 'string' ? JSON.parse(toolPart.args) : toolPart.args;
              if (args.sub_session_id) {
                return (
                  <SubagentCard
                    key={index}
                    subSessionId={args.sub_session_id}
                    agentName={args.agent_name || 'Subagent'}
                    client={client}
                    parentSessionId={sessionId}
                  />
                );
              }
            }
            return <ToolCallCard key={index} part={toolPart} />;
          }
          default:
            return null;
        }
      })}
    </>
  );
};

const ChatPage: React.FC = () => {
  const client = useClient();
  const { token } = useToken();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeKey, setActiveKey] = useState<string>('');
  const [loadingSessions, setLoadingSessions] = useState(true);
  const [todos, setTodos] = useState<Todo[]>([]);
  const [sidebarVisible, setSidebarVisible] = useState(true);
  const [sidebarTab, setSidebarTab] = useState<'tasks' | 'memory'>('tasks');

  const { messages, status, sendMessage, abort } = useChat({
    client,
    sessionId: activeKey,
  });

  const isRequesting = status === 'streaming';

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
      message.error('Failed to load sessions');
    } finally {
      setLoadingSessions(false);
    }
  };

  const loadTodos = async (sessionId: string) => {
    try {
      const result = await client.getTodos(sessionId);
      setTodos(result.todos || []);
    } catch (error) {
      message.error('Failed to load todos');
      setTodos([]);
    }
  };

  const handleCreateSession = async () => {
    try {
      const session = await client.createSession();
      setSessions([session, ...sessions]);
      setActiveKey(session.id);
      message.success('New session created');
    } catch (error) {
      message.error('Failed to create session');
    }
  };

  const handleDeleteSession = async (key: string) => {
    try {
      await client.deleteSession(key);
      const remaining = sessions.filter((s) => s.id !== key);
      setSessions(remaining);
      if (activeKey === key) {
        setActiveKey(remaining.length > 0 ? remaining[0].id : '');
      }
      message.success('Session deleted');
    } catch (error) {
      message.error('Failed to delete session');
    }
  };

  const handleTodoStatusChange = (id: string, newStatus: string) => {
    setTodos((prev) =>
      prev.map((todo) => (todo.id === id ? { ...todo, status: newStatus as Todo['status'] } : todo))
    );
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
            <StepContent step={step} sessionId={activeKey} />
          </React.Fragment>
        ))}
      </>
    );
  };

  const bubbleItems: BubbleItem[] = [];

  messages.forEach((msg: CopConMessage) => {
    const isLastAssistant =
      msg.role === 'assistant' && messages.indexOf(msg) === messages.length - 1;

    if (msg.role === 'user') {
      const userText = msg.steps[0]?.parts.find((p) => p.type === 'text')?.text || '';
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
        loading:
          isLastAssistant &&
          isRequesting &&
          !msg.steps.some((s) =>
            s.parts.some(
              (p) =>
                (p.type === 'text' && p.text) ||
                (p.type === 'reasoning' && p.text) ||
                p.type === 'tool-call'
            )
          ),
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

  return (
    <Flex
      style={{
        height: '100%',
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
          <Button type="primary" size="small" onClick={handleCreateSession} aria-label="Create new session">
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
                    icon: <span aria-hidden="true">🗑️</span>,
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
                onCancel={async () => {
                  if (activeKey) {
                    try {
                      await client.stop(activeKey);
                    } catch {}
                  }
                  abort();
                }}
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
            <Segmented
              value={sidebarTab}
              onChange={(value) => setSidebarTab(value as 'tasks' | 'memory')}
              options={[
                { value: 'tasks', icon: <CheckSquareOutlined />, label: 'Tasks' },
                { value: 'memory', icon: <DatabaseOutlined />, label: 'Memory' },
              ]}
              size="small"
            />
            <Button
              type="text"
              size="small"
              icon={<MenuFoldOutlined />}
              onClick={() => setSidebarVisible(false)}
              aria-label="Collapse sidebar"
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
            {sidebarTab === 'tasks' ? (
              <TodoList todos={todos} onStatusChange={handleTodoStatusChange} />
            ) : (
              (() => {
                const currentSession = sessions.find((s) => s.id === activeKey);
                const currentAgentId = currentSession?.default_agent_id || '';
                return <MemoryPanel agentId={currentAgentId} limit={5} />;
              })()
            )}
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
          aria-label="Expand sidebar"
        />
      )}
    </Flex>
  );
};

export default ChatPage;
