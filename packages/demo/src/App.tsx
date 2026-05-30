import React, { useState } from 'react';
import { XProvider } from '@ant-design/x';
import { Tabs, theme } from 'antd';
import { MessageOutlined, BookOutlined, DatabaseOutlined } from '@ant-design/icons';
import { AgentClient } from '@copcon/chat-core';
import { ClientProvider } from './context/ClientContext';
import { ErrorBoundary } from './components/ErrorBoundary';
import ChatPage from './pages/ChatPage';
import KnowledgePage from './pages/KnowledgePage';
import MemoryPage from './pages/MemoryPage';
import './App.css';

const { useToken } = theme;

const client = new AgentClient({ baseUrl: '' });

type TabKey = 'chat' | 'knowledge' | 'memory';

const App: React.FC = () => {
  const { token } = useToken();
  const [activeTab, setActiveTab] = useState<TabKey>('chat');

  const tabItems = [
    {
      key: 'chat' as TabKey,
      label: 'Chat',
      icon: <MessageOutlined aria-hidden="true" />,
      children: (
        <ErrorBoundary>
          <ChatPage />
        </ErrorBoundary>
      ),
    },
    {
      key: 'knowledge' as TabKey,
      label: 'Knowledge Base',
      icon: <BookOutlined aria-hidden="true" />,
      children: (
        <ErrorBoundary>
          <KnowledgePage />
        </ErrorBoundary>
      ),
    },
    {
      key: 'memory' as TabKey,
      label: 'Memory',
      icon: <DatabaseOutlined aria-hidden="true" />,
      children: (
        <ErrorBoundary>
          <MemoryPage />
        </ErrorBoundary>
      ),
    },
  ];

  return (
    <ClientProvider client={client}>
      <XProvider>
        <Tabs
          activeKey={activeTab}
          onChange={(key) => setActiveTab(key as TabKey)}
          items={tabItems.map((item) => ({
            key: item.key,
            label: (
              <span>
                {item.icon}
                {item.label}
              </span>
            ),
            children: (
              <div
                style={{
                  height: 'calc(100vh - 48px)',
                  overflow: 'hidden',
                  background: token.colorBgLayout,
                }}
              >
                {item.children}
              </div>
            ),
          }))}
          style={{
            height: '100vh',
            display: 'flex',
            flexDirection: 'column',
          }}
          tabBarStyle={{
            marginBottom: 0,
            paddingLeft: token.padding,
            paddingRight: token.padding,
            background: token.colorBgContainer,
            borderBottom: `1px solid ${token.colorBorderSecondary}`,
          }}
        />
      </XProvider>
    </ClientProvider>
  );
};

export default App;
