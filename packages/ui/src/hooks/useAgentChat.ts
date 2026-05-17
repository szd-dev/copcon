import { useState, useCallback, useRef, useEffect } from 'react';
import { useXChat, XRequest } from '@ant-design/x-sdk';
import type { MessageInfo } from '@ant-design/x-sdk';
import CopConChatProvider, {
  CopConMessage,
  CopConInput,
  CopConSSEOutput,
} from '../providers/CopConChatProvider';
import { AgentClient } from '../api/agentClient';


export interface UseAgentChatOptions {
  /** AgentClient instance for API calls */
  client: AgentClient;
  /** Current session ID */
  sessionId: string;
}

export interface UseAgentChatReturn {
  /** Array of chat messages */
  messages: CopConMessage[];
  /** Whether a request is in progress */
  isRequesting: boolean;
  /** Send a new message */
  sendMessage: (content: string) => void;
  /** Abort the current request */
  abort: () => void;
}

/**
 * Custom hook that wraps @ant-design/x-sdk's useXChat for CopCon chat functionality.
 *
 * Uses CopConChatProvider to handle SSE message transformation and useXChat for
 * state management.
 */
export function useAgentChat(options: UseAgentChatOptions): UseAgentChatReturn {
  const { client, sessionId } = options;

  const [provider, setProvider] = useState<CopConChatProvider | null>(null);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);
  const loadedSessionRef = useRef<string | null>(null);

  // Create provider when sessionId changes
  useEffect(() => {
    if (!sessionId) {
      setProvider(null);
      loadedSessionRef.current = null;
      return;
    }

    const baseUrl = client.getBaseUrl();
    const request = XRequest<CopConInput, CopConSSEOutput, CopConMessage>(
      `${baseUrl}/api/sessions/${sessionId}/chat`,
      {
        manual: true,
        params: { content: '', sessionId },
      }
    );

    setProvider(new CopConChatProvider({ request }));
  }, [client, sessionId]);

  const chatResult = useXChat<CopConMessage, CopConMessage, CopConInput, CopConSSEOutput>({
    provider: provider ?? undefined,
  });

  // Load historical messages when sessionId changes
  useEffect(() => {
    if (!sessionId || !client || loadedSessionRef.current === sessionId) {
      return;
    }

    const loadMessages = async () => {
      setIsLoadingMessages(true);
      try {
        const result = await client.getMessages(sessionId);
        const messages = result.messages || [];
        const messageInfos: MessageInfo<CopConMessage>[] = messages.map(
          (msg) => ({
            id: msg.id,
            message: msg,
            status: 'success' as const,
          })
        );
        chatResult.setMessages(messageInfos);
        loadedSessionRef.current = sessionId;
      } catch (error) {
        console.error('Failed to load messages:', error);
        chatResult.setMessages([]);
      } finally {
        setIsLoadingMessages(false);
      }
    };

    loadMessages();
  }, [client, sessionId, chatResult]);

  const sendMessage = useCallback(
    (content: string) => {
      if (!sessionId || !provider) return;

      chatResult.onRequest({ content, sessionId });
    },
    [sessionId, provider, chatResult]
  );

  return {
    messages: chatResult.messages.map((m) => m.message),
    isRequesting: chatResult.isRequesting || isLoadingMessages,
    sendMessage,
    abort: chatResult.abort,
  };
}
