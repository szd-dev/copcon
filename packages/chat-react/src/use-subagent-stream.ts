import { useState, useEffect, useRef } from 'react';
import { SubagentStream, AgentClient, CopConMessage } from '@copcon/chat-core';

interface UseSubagentStreamOptions {
  client: AgentClient;
  sessionId: string;
}

interface UseSubagentStreamReturn {
  messages: CopConMessage[];
  isStreaming: boolean;
  error: Error | undefined;
}

export function useSubagentStream(options: UseSubagentStreamOptions): UseSubagentStreamReturn {
  const { client, sessionId } = options;
  const [messages, setMessages] = useState<CopConMessage[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<Error | undefined>(undefined);
  const streamRef = useRef<SubagentStream | null>(null);

  useEffect(() => {
    if (!sessionId) return;

    const stream = new SubagentStream({
      client,
      sessionId,
      callbacks: {
        onMessagesChange: (msgs) => setMessages(msgs),
        onStreamingChange: (streaming) => setIsStreaming(streaming),
        onError: (err) => setError(err),
      },
    });
    streamRef.current = stream;
    stream.start();

    return () => {
      stream.destroy();
      streamRef.current = null;
    };
  }, [client, sessionId]);

  return { messages, isStreaming, error };
}