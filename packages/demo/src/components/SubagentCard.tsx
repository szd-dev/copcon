import { useState } from 'react';
import { useSubagentStream } from '@copcon/chat-react';
import { AgentClient, Part } from '@copcon/chat-core';
import { ThinkingBlock } from './ThinkingBlock';
import { ToolCallCard } from './ToolCallCard';
import { HumanInteraction } from './HumanInteraction';
import { StreamMarkdown } from './StreamMarkdown';

interface SubagentCardProps {
  subSessionId: string;
  agentName?: string;
  autoExpand?: boolean;
  client: AgentClient;
  parentSessionId: string;
}

export function SubagentCard({
  subSessionId,
  agentName = 'Subagent',
  autoExpand = false,
  client,
  parentSessionId,
}: SubagentCardProps) {
  const [expanded, setExpanded] = useState(autoExpand);
  const { messages, isStreaming, error } = useSubagentStream({
    client,
    sessionId: subSessionId,
  });

  const statusText = isStreaming ? 'Streaming' : error ? 'Error' : 'Complete';
  const statusColor = isStreaming ? '#1677ff' : error ? '#ff4d4f' : '#52c41a';

  const renderPart = (part: Part) => {
    switch (part.type) {
      case 'text':
        return <StreamMarkdown content={part.text} />;
      case 'reasoning':
        return <ThinkingBlock part={part} />;
      case 'tool-call':
        if (part.state === 'waiting_for_input' && part.interrupt) {
          return (
            <HumanInteraction
              part={part}
              sessionId={parentSessionId}
              client={client}
            />
          );
        }
        return <ToolCallCard part={part} />;
      default:
        return null;
    }
  };

  return (
    <div style={{ border: '1px solid #e8e8e8', borderRadius: 8, marginBottom: 8 }}>
      <div
        onClick={() => setExpanded(!expanded)}
        style={{
          padding: '8px 12px',
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          userSelect: 'none',
        }}
      >
        <span>{expanded ? '▼' : '▶'}</span>
        <span style={{ fontWeight: 500 }}>{agentName}</span>
        <span style={{ color: statusColor, fontSize: 12 }}>{statusText}</span>
      </div>
      {expanded && (
        <div style={{ padding: '0 12px 12px' }}>
          {error && <div style={{ color: '#ff4d4f', marginBottom: 8 }}>Error: {error.message}</div>}
          {messages.map((msg, msgIdx) => (
            <div key={msg.id || msgIdx}>
              {msg.steps.map((step, stepIdx) => (
                <div key={stepIdx}>
                  {step.parts.map((part, partIdx) => (
                    <div key={partIdx}>{renderPart(part)}</div>
                  ))}
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}