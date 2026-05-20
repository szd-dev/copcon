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
  /** Whether reconnecting after an SSE disconnect */
  isReconnecting: boolean;
  /** Send a new message */
  sendMessage: (content: string) => void;
  /** Abort the current request */
  abort: () => void;
}

export function useAgentChat(options: UseAgentChatOptions): UseAgentChatReturn {
  const { client, sessionId } = options;

  const [provider, setProvider] = useState<CopConChatProvider | null>(null);
  const [isLoadingMessages, setIsLoadingMessages] = useState(false);
  const [isReconnecting, setIsReconnecting] = useState(false);
  const loadedSessionRef = useRef<string | null>(null);

  const lastReceivedSeqRef = useRef(0);
  const isStreamCompleteRef = useRef(false);
  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;

  const providerRef = useRef<CopConChatProvider | null>(null);
  providerRef.current = provider;
  const isReconnectingRef = useRef(false);
  isReconnectingRef.current = isReconnecting;

  const chatResultRef = useRef<{
    messages: MessageInfo<CopConMessage>[];
    setMessages: (msgs: MessageInfo<CopConMessage>[] | ((ori: MessageInfo<CopConMessage>[]) => MessageInfo<CopConMessage>[])) => boolean;
    isRequesting: boolean;
    abort: () => void;
    isDefaultMessagesRequesting: boolean;
  } | null>(null);

  const handleReconnectRef = useRef<() => Promise<void>>(async () => {});

  useEffect(() => {
    if (!sessionId) {
      setProvider(null);
      providerRef.current = null;
      loadedSessionRef.current = null;
      lastReceivedSeqRef.current = 0;
      isStreamCompleteRef.current = false;
      setIsReconnecting(false);
      return;
    }

    setIsReconnecting(false);
    lastReceivedSeqRef.current = 0;
    isStreamCompleteRef.current = false;

    const baseUrl = client.getBaseUrl();
    const request = XRequest<CopConInput, CopConSSEOutput, CopConMessage>(
      `${baseUrl}/api/sessions/${sessionId}/chat`,
      {
        manual: true,
        params: { content: '', sessionId },
        callbacks: {
          onSuccess: () => {},
          onUpdate: (chunk: CopConSSEOutput, _headers: Headers) => {
            lastReceivedSeqRef.current += 1;
            if (chunk?.data) {
              try {
                const parsed = JSON.parse(chunk.data);
                if (parsed.type === 'message_done') {
                  isStreamCompleteRef.current = true;
                }
              } catch {
              }
            }
          },
          onError: async (error: Error) => {
            if (error.name === 'AbortError') return;
            if (isReconnectingRef.current) return;
            await handleReconnectRef.current();
          },
        },
      }
    );

    const newProvider = new CopConChatProvider({ request });
    setProvider(newProvider);
    providerRef.current = newProvider;
  }, [client, sessionId]);

  const chatResult = useXChat<CopConMessage, CopConMessage, CopConInput, CopConSSEOutput>({
    provider: provider ?? undefined,
  });

  chatResultRef.current = chatResult;

  const handleReconnect = useCallback(async () => {
    const currentSessionId = sessionIdRef.current;
    if (!currentSessionId || !client) return;

    setIsReconnecting(true);
    isReconnectingRef.current = true;

    try {
      const response = await client.reconnect(currentSessionId, lastReceivedSeqRef.current + 1);

      if (response.status === 204) {
        await refreshMessagesFromAPI(currentSessionId);
        setIsReconnecting(false);
        isReconnectingRef.current = false;
        return;
      }

      if (!response.body) {
        throw new Error('Reconnect response has no body');
      }

      const currentProvider = providerRef.current;
      if (!currentProvider) throw new Error('No provider available for transform');

      await parseReconnectSSE(response, currentProvider);

      setIsReconnecting(false);
      isReconnectingRef.current = false;
    } catch {
      try {
        await refreshMessagesAndCreateFreshProvider(currentSessionId);
      } catch (fetchError) {
        console.error('[useAgentChat] Failed to fetch messages after reconnect failure:', fetchError);
      }
      setIsReconnecting(false);
      isReconnectingRef.current = false;
    }
  }, [client]);

  handleReconnectRef.current = handleReconnect;

  const refreshMessagesFromAPI = async (sid: string) => {
    const result = await client.getMessages(sid);
    const messages = result.messages || [];
    const messageInfos: MessageInfo<CopConMessage>[] = messages.map(
      (msg) => ({
        id: msg.id,
        message: msg,
        status: 'success' as const,
      })
    );
    chatResultRef.current?.setMessages(messageInfos);
  };

  const parseReconnectSSE = async (
    response: Response,
    currentProvider: CopConChatProvider,
  ) => {
    const reader = response.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      let currentData = '';

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          currentData += (currentData ? '\n' : '') + line.slice(6);
        } else if (line === '' && currentData) {
          applySSEChunk(currentData, currentProvider);
          currentData = '';
        }
      }
    }

    const remainder = buffer.trim();
    if (remainder.startsWith('data: ')) {
      applySSEChunk(remainder.slice(6), currentProvider);
    }
  };

  const applySSEChunk = (rawData: string, currentProvider: CopConChatProvider) => {
    const chunk: CopConSSEOutput = { data: rawData };

    lastReceivedSeqRef.current += 1;

    try {
      const parsed = JSON.parse(rawData);
      if (parsed.type === 'message_done') {
        isStreamCompleteRef.current = true;
      }
    } catch {
    }

    chatResultRef.current?.setMessages(
      (ori: MessageInfo<CopConMessage>[]) => {
        return ori.map((info) => {
          if (info.status === 'loading' || info.status === 'updating') {
            const transformed = currentProvider.transformMessage({
              originMessage: info.message,
              chunk,
              chunks: [],
              status: 'updating',
              responseHeaders: new Headers(),
            });
            return { ...info, message: transformed, status: 'updating' as const };
          }
          return info;
        });
      }
    );
  };

  const refreshMessagesAndCreateFreshProvider = async (sid: string) => {
    const result = await client.getMessages(sid);
    const fetchedMessages = result.messages || [];

    const currentMessages = chatResultRef.current?.messages ?? [];
    const fetchedIdSet = new Set(fetchedMessages.map((m) => m.id));

    const merged: MessageInfo<CopConMessage>[] = fetchedMessages.map((msg) => ({
      id: msg.id,
      message: msg,
      status: 'success' as const,
    }));

    for (const msg of currentMessages) {
      if (!fetchedIdSet.has(msg.id as string)) {
        merged.push(msg);
      }
    }

    chatResultRef.current?.setMessages(merged);

    const baseUrl = client.getBaseUrl();
    const request = XRequest<CopConInput, CopConSSEOutput, CopConMessage>(
      `${baseUrl}/api/sessions/${sid}/chat`,
      {
        manual: true,
        params: { content: '', sessionId: sid },
        callbacks: {
          onSuccess: () => {},
          onUpdate: (chunk: CopConSSEOutput, _headers: Headers) => {
            lastReceivedSeqRef.current += 1;
            if (chunk?.data) {
              try {
                const parsed = JSON.parse(chunk.data);
                if (parsed.type === 'message_done') {
                  isStreamCompleteRef.current = true;
                }
              } catch {
              }
            }
          },
          onError: async (error: Error) => {
            if (error.name === 'AbortError') return;
            if (isReconnectingRef.current) return;
            await handleReconnectRef.current();
          },
        },
      }
    );

    const freshProvider = new CopConChatProvider({ request });
    setProvider(freshProvider);
    providerRef.current = freshProvider;
    lastReceivedSeqRef.current = 0;
    isStreamCompleteRef.current = false;
  };

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
    isReconnecting,
    sendMessage,
    abort: chatResult.abort,
  };
}