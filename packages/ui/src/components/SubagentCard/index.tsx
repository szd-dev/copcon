import React, { useState, useCallback } from 'react';
import { Think, ThoughtChain } from '@ant-design/x';
import { XMarkdown } from '@ant-design/x-markdown';
import { Divider, Flex, Typography, Badge, theme } from 'antd';
import { DownOutlined, RightOutlined } from '@ant-design/icons';
import { useSubagentSSE } from '../../hooks/useSubagentSSE';
import type { Step, ToolCallPart } from '../../api/types';
import type { CopConMessage } from '../../providers/CopConChatProvider';

const { useToken } = theme;
const { Text } = Typography;

const mapToolCallStatus = (state: string): 'loading' | 'success' | 'error' => {
  switch (state) {
    case 'pending':
      return 'loading';
    case 'running':
      return 'loading';
    case 'complete':
      return 'success';
    case 'error':
      return 'error';
    default:
      return 'loading';
  }
};

const MarkdownContent: React.FC<{ content: string }> = ({ content }) => (
  <XMarkdown content={content} />
);

const StepContent: React.FC<{ step: Step }> = ({ step }) => {
  const toolCallParts: ToolCallPart[] = [];

  return (
    <>
      {step.parts.map((part, index) => {
        switch (part.type) {
          case 'text':
            return <MarkdownContent key={index} content={part.text} />;
          case 'reasoning':
            return (
              <Think key={index} title="Thinking" defaultExpanded>
                <MarkdownContent content={part.text} />
              </Think>
            );
          case 'tool-call':
            toolCallParts.push(part);
            return null;
          default:
            return null;
        }
      })}
      {toolCallParts.length > 0 && (
        <ThoughtChain
          items={toolCallParts.map((part) => ({
            key: part.toolCallId,
            title: part.toolName,
            status: mapToolCallStatus(part.state),
            description: part.args,
            content: part.output ? (
              <MarkdownContent content={part.output} />
            ) : undefined,
          }))}
        />
      )}
    </>
  );
};

export interface SubagentCardProps {
  subSessionId: string;
  agentName?: string;
  autoExpand?: boolean;
}

export const SubagentCard: React.FC<SubagentCardProps> = ({
  subSessionId,
  agentName,
  autoExpand = false,
}) => {
  const { token } = useToken();
  const [expanded, setExpanded] = useState(autoExpand);
  const { messages, isStreaming, error } = useSubagentSSE({
    sessionId: subSessionId,
  });

  const handleToggle = useCallback(() => {
    setExpanded((prev) => !prev);
  }, []);

  const renderMessageContent = (msg: CopConMessage) => {
    if (!msg.steps || msg.steps.length === 0) {
      return null;
    }

    return (
      <>
        {msg.steps.map((step, stepIndex) => (
          <React.Fragment key={stepIndex}>
            {stepIndex > 0 && <Divider style={{ margin: '8px 0' }} />}
            <StepContent step={step} />
          </React.Fragment>
        ))}
      </>
    );
  };

  const statusText = isStreaming ? 'Streaming' : error ? 'Error' : 'Complete';
  const badgeStatus = isStreaming
    ? 'processing'
    : error
      ? 'error'
      : 'default';

  return (
    <div
      style={{
        border: `1px solid ${token.colorBorder}`,
        borderRadius: token.borderRadiusLG,
        overflow: 'hidden',
        background: token.colorBgContainer,
      }}
    >
      <div
        onClick={handleToggle}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            handleToggle();
          }
        }}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        style={{
          padding: `${token.paddingSM}px ${token.padding}px`,
          background: token.colorBgLayout,
          cursor: 'pointer',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          userSelect: 'none',
        }}
      >
        <Flex align="center" gap="small">
          <Badge status={badgeStatus} />
          <Text strong>{agentName || 'Subagent'}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {statusText}
          </Text>
        </Flex>
        {expanded ? (
          <DownOutlined
            style={{ color: token.colorTextSecondary, fontSize: 12 }}
          />
        ) : (
          <RightOutlined
            style={{ color: token.colorTextSecondary, fontSize: 12 }}
          />
        )}
      </div>

      {expanded && (
        <div style={{ padding: token.padding }}>
          {error && (
            <Text
              type="danger"
              style={{ display: 'block', marginBottom: token.marginSM }}
            >
              {error.message}
            </Text>
          )}
          {messages.map((msg, index) => (
            <div
              key={msg.id || index}
              style={{ marginBottom: token.marginMD }}
            >
              {renderMessageContent(msg)}
            </div>
          ))}
          {messages.length === 0 && !error && isStreaming && (
            <Text type="secondary">Waiting for subagent...</Text>
          )}
          {messages.length === 0 && !error && !isStreaming && (
            <Text type="secondary">No messages</Text>
          )}
        </div>
      )}
    </div>
  );
};

export default SubagentCard;
