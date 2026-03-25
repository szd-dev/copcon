import { useState, useCallback, useRef, useEffect } from 'react';
import { AgentClient } from '../api/agentClient';
import { Message, SSEEvent, ToolExecution } from '../api/types';

export interface UseAgentChatOptions {
  client: AgentClient;
  sessionId: string;
}

export interface UseAgentChatReturn {
  messages: Message[];
  isLoading: boolean;
  toolExecutions: ToolExecution[];
  sendMessage: (content: string) => void;
  stopGeneration: () => void;
  loadMessages: () => Promise<void>;
}

export function useAgentChat({ client, sessionId }: UseAgentChatOptions): UseAgentChatReturn {
  const [messages, setMessages] = useState<Message[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [toolExecutions, setToolExecutions] = useState<ToolExecution[]>([]);
  const stopRef = useRef<(() => void) | null>(null);
  const currentToolRef = useRef<ToolExecution | null>(null);
  const currentReasoningRef = useRef('');

  const loadMessages = useCallback(async () => {
    if (!sessionId) {
      setMessages([]);
      return;
    }
    try {
      const result = await client.getMessages(sessionId);
      setMessages(result.messages || []);
    } catch (error) {
      console.error('Failed to load messages:', error);
      setMessages([]);
    }
  }, [client, sessionId]);

  useEffect(() => {
    setMessages([]);
    if (sessionId) {
      loadMessages();
    }
  }, [sessionId]);

  const handleEvent = useCallback((event: SSEEvent) => {
    console.log('[useAgentChat] Received event:', event.type, event.data);
    
    switch (event.type) {
      case 'reasoning':
        if (event.data.content) {
          console.log('[useAgentChat] Reasoning content:', event.data.content);
          currentReasoningRef.current += event.data.content;
        }
        break;
      case 'message':
        setMessages((prev) => {
          const last = prev[prev.length - 1];
          if (last?.role === 'assistant') {
            return [...prev.slice(0, -1), { 
              ...last, 
              content: last.content + (event.data.content || ''),
              reasoning: last.reasoning || currentReasoningRef.current,
            }];
          }
          return [...prev, {
            id: `temp-${Date.now()}`,
            session_id: sessionId,
            role: 'assistant' as const,
            content: event.data.content || '',
            reasoning: currentReasoningRef.current,
            created_at: new Date().toISOString(),
          }];
        });
        break;
      case 'tool_call': {
        const newTool: ToolExecution = {
          id: event.data.id || `tool-${Date.now()}`,
          name: event.data.name || event.data.tool_name || 'Unknown Tool',
          arguments: event.data.arguments ? JSON.parse(event.data.arguments) : (event.data.tool_args || {}),
          status: 'running',
          startTime: Date.now(),
        };
        currentToolRef.current = newTool;
        setToolExecutions((prev) => [...prev, newTool]);
        break;
      }
      case 'tool_result': {
        setToolExecutions((prev) => {
          const toolId = event.data.id || currentToolRef.current?.id;
          if (!toolId) return prev;
          
          return prev.map((tool) => {
            if (tool.id === toolId) {
              return {
                ...tool,
                output: typeof event.data.output === 'string' 
                  ? event.data.output 
                  : JSON.stringify(event.data.output || event.data.result, null, 2),
                status: 'success',
                endTime: Date.now(),
              };
            }
            return tool;
          });
        });
        currentToolRef.current = null;
        break;
      }
      case 'thought':
        if (event.data.content) {
          setMessages((prev) => {
            const last = prev[prev.length - 1];
            if (last?.role === 'assistant') {
              return [...prev.slice(0, -1), { ...last, content: last.content + `\n\n> ${event.data.content}` }];
            }
            return [...prev, {
              id: `temp-${Date.now()}`,
              session_id: sessionId,
              role: 'assistant' as const,
              content: `> ${event.data.content}`,
              created_at: new Date().toISOString(),
            }];
          });
        }
        break;
      case 'done':
        setIsLoading(false);
        stopRef.current = null;
        currentToolRef.current = null;
        currentReasoningRef.current = '';
        break;
      case 'error':
        setIsLoading(false);
        stopRef.current = null;
        currentToolRef.current = null;
        currentReasoningRef.current = '';
        console.error('Chat error:', event.data.error);
        break;
    }
  }, [sessionId]);

  const sendMessage = useCallback((content: string) => {
    if (!sessionId) return;
    
    setToolExecutions([]);
    currentToolRef.current = null;
    currentReasoningRef.current = '';
    
    setMessages((prev) => [...prev, {
      id: `user-${Date.now()}`,
      session_id: sessionId,
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    }]);
    
    setIsLoading(true);
    stopRef.current = client.chat(sessionId, content, handleEvent);
  }, [client, sessionId, handleEvent]);

  const stopGeneration = useCallback(() => {
    if (stopRef.current) {
      stopRef.current();
      stopRef.current = null;
      setIsLoading(false);
    }
  }, []);

  return { messages, isLoading, toolExecutions, sendMessage, stopGeneration, loadMessages };
}