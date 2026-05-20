import { useState, useEffect, useRef } from 'react';
import { XRequest } from '@ant-design/x-sdk';
import CopConChatProvider, {
  CopConMessage,
  CopConInput,
  CopConSSEOutput,
} from '../providers/CopConChatProvider';
import { AgentClient } from '../api/agentClient';

export interface UseSubagentSSEOptions {
  sessionId: string;
  client?: AgentClient;
}

export interface UseSubagentSSEReturn {
  messages: CopConMessage[];
  isStreaming: boolean;
  error: Error | null;
}

interface ReconnectInput {
  content: string;
  sessionId: string;
  agentId?: string;
  reconnect: boolean;
  last_event_seq: number;
}

export function useSubagentSSE(options: UseSubagentSSEOptions): UseSubagentSSEReturn {
  const { sessionId, client } = options;
  const [messages, setMessages] = useState<CopConMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const messagesRef = useRef<CopConMessage[]>([]);
  const currentMessageRef = useRef<CopConMessage | null>(null);
  const providerRef = useRef<CopConChatProvider | null>(null);

  useEffect(() => {
    if (!sessionId) {
      setMessages([]);
      setIsStreaming(false);
      setError(null);
      messagesRef.current = [];
      currentMessageRef.current = null;
      return;
    }

    setMessages([]);
    setError(null);
    setIsStreaming(true);
    messagesRef.current = [];
    currentMessageRef.current = null;

    const dummyRequest = XRequest<CopConInput, CopConSSEOutput, CopConMessage>('', {
      manual: true,
    });
    const provider = new CopConChatProvider({ request: dummyRequest });
    providerRef.current = provider;

    const baseUrl = client?.getBaseUrl() || '';
    const request = XRequest<ReconnectInput, CopConSSEOutput, CopConMessage>(
      `${baseUrl}/api/sessions/${sessionId}/chat`,
      {
        manual: true,
        params: {
          content: '',
          sessionId,
          reconnect: true,
          last_event_seq: 0,
        },
        callbacks: {
          onSuccess: () => {
            setIsStreaming(false);
            if (currentMessageRef.current) {
              messagesRef.current = [...messagesRef.current, currentMessageRef.current];
              currentMessageRef.current = null;
              setMessages([...messagesRef.current]);
            }
          },
          onError: (err: Error) => {
            if (err.name === 'AbortError') {
              setIsStreaming(false);
              return;
            }
            setError(err);
            setIsStreaming(false);
          },
          onUpdate: (chunk: CopConSSEOutput) => {
            try {
              const transformed = provider.transformMessage({
                originMessage: currentMessageRef.current || undefined,
                chunk,
                chunks: [],
                status: 'updating',
                responseHeaders: new Headers(),
              });

              currentMessageRef.current = transformed;

              let isDone = false;
              if (chunk?.data) {
                try {
                  const parsed = JSON.parse(chunk.data);
                  if (parsed.type === 'message_done') {
                    isDone = true;
                  }
                } catch {
                }
              }

              if (isDone) {
                messagesRef.current = [...messagesRef.current, transformed];
                currentMessageRef.current = null;
                setMessages([...messagesRef.current]);
              } else {
                setMessages([...messagesRef.current, transformed]);
              }
            } catch (transformErr) {
              console.error('[useSubagentSSE] Transform error:', transformErr);
            }
          },
        },
      },
    );

    request.run({
      content: '',
      sessionId,
      reconnect: true,
      last_event_seq: 0,
    });

    return () => {
      request.abort();
    };
  }, [sessionId, client]);

  return { messages, isStreaming, error };
}
