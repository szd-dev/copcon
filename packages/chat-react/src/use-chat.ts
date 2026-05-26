import { useSyncExternalStore, useEffect, useCallback, useRef } from 'react';
import { ChatSession, AgentClient, CopConMessage } from '@copcon/chat-core';
import { ReactChatState } from './react-chat-state';

interface UseChatOptions {
  client: AgentClient;
  sessionId: string;
}

interface UseChatReturn {
  messages: CopConMessage[];
  status: string;
  error: Error | undefined;
  sendMessage: (content: string) => void;
  abort: () => void;
}

export function useChat(options: UseChatOptions): UseChatReturn {
  const { client, sessionId } = options;
  const chatStateRef = useRef(new ReactChatState());
  const sessionRef = useRef<ChatSession | null>(null);

  useEffect(() => {
    if (!sessionId) return;

    const chatState = chatStateRef.current;
    const session = new ChatSession({
      client,
      sessionId,
      callbacks: {
        onMessagesChange: (msgs) => chatState.setMessages(msgs),
        onStateChange: (state) => chatState.setState(state),
      },
    });
    sessionRef.current = session;
    session.start();

    return () => {
      session.destroy();
      sessionRef.current = null;
    };
  }, [client, sessionId]);

  const snapshot = useSyncExternalStore(
    (callback) => chatStateRef.current.subscribe(callback),
    () => chatStateRef.current.getSnapshot(),
  );

  const sendMessage = useCallback((content: string) => {
    sessionRef.current?.sendMessage(content);
  }, []);

  const abort = useCallback(() => {
    sessionRef.current?.abort();
  }, []);

  return {
    messages: snapshot.messages,
    status: snapshot.state.status,
    error: snapshot.state.error,
    sendMessage,
    abort,
  };
}