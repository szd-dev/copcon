import { useMemo } from 'react';
import { createToolCallController } from '@copcon/headless-hooks';
import type { ToolCallPart } from '@copcon/chat-core';
import { StreamMarkdown } from './StreamMarkdown';

interface ToolCallCardProps {
  part: ToolCallPart;
}

const STATUS_COLORS: Record<string, string> = {
  pending: '#faad14',
  running: '#1677ff',
  complete: '#52c41a',
  error: '#ff4d4f',
  waiting_for_input: '#722ed1',
};

const STATUS_LABELS: Record<string, string> = {
  pending: 'Pending',
  running: 'Running',
  complete: 'Complete',
  error: 'Error',
  waiting_for_input: 'Waiting',
};

export function ToolCallCard({ part }: ToolCallCardProps) {
  const controller = useMemo(() => createToolCallController(part), [part]);

  return (
    <div style={{ marginBottom: 8, padding: 8, borderRadius: 6, border: '1px solid #f0f0f0', background: '#fafafa' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ fontWeight: 600 }}>{controller.toolName}</span>
        <span
          style={{
            padding: '1px 8px',
            borderRadius: 4,
            fontSize: 12,
            color: '#fff',
            background: STATUS_COLORS[controller.status] || '#999',
          }}
        >
          {STATUS_LABELS[controller.status] || controller.status}
        </span>
      </div>

      {controller.parsedArgs != null && (
        <details style={{ marginTop: 4 }}>
          <summary style={{ cursor: 'pointer', fontSize: 12, color: '#666' }}>Arguments</summary>
          <pre style={{ fontSize: 12, overflow: 'auto', maxHeight: 200 }}>
            {typeof controller.parsedArgs === 'string'
              ? String(controller.parsedArgs)
              : JSON.stringify(controller.parsedArgs, null, 2)}
          </pre>
        </details>
      )}

      {controller.parsedOutput.output && (
        <div style={{ marginTop: 4 }}>
          <div style={{ fontSize: 12, color: '#666' }}>Output:</div>
          <StreamMarkdown content={controller.parsedOutput.output} />
        </div>
      )}

      {controller.error && (
        <div style={{ marginTop: 4, color: '#ff4d4f', fontSize: 13 }}>
          Error: {controller.error}
        </div>
      )}
    </div>
  );
}