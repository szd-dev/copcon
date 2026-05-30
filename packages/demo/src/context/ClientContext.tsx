import React, { createContext, useContext } from 'react';
import { AgentClient } from '@copcon/chat-core';

const ClientContext = createContext<AgentClient | null>(null);

export function ClientProvider({ children, client }: { children: React.ReactNode; client: AgentClient }) {
  return <ClientContext.Provider value={client}>{children}</ClientContext.Provider>;
}

export function useClient(): AgentClient {
  const client = useContext(ClientContext);
  if (!client) {
    throw new Error('useClient must be used within a ClientProvider');
  }
  return client;
}
