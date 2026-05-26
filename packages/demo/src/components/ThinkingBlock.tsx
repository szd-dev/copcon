import { useMemo, useState } from 'react';
import { createThinkingChainController } from '@copcon/headless-hooks';
import type { ReasoningPart } from '@copcon/chat-core';
import { StreamMarkdown } from './StreamMarkdown';

interface ThinkingBlockProps {
  part: ReasoningPart;
}

export function ThinkingBlock({ part }: ThinkingBlockProps) {
  const controller = useMemo(() => createThinkingChainController(part), [part]);
  const [expanded, setExpanded] = useState(controller.expanded);

  const handleToggle = () => {
    controller.toggle();
    setExpanded(!expanded);
  };

  const containerProps = controller.getContainerProps();

  return (
    <div {...containerProps} style={{ marginBottom: 8 }}>
      <div
        onClick={handleToggle}
        style={{ cursor: 'pointer', userSelect: 'none', fontWeight: 500, color: '#888' }}
        {...controller.getToggleProps()}
      >
        {expanded ? '▼' : '▶'} Thinking{controller.isStreaming ? '...' : ''}
      </div>
      {!controller.getContentProps().hidden && (
        <div style={{ paddingLeft: 16, borderLeft: '2px solid #e8e8e8', marginTop: 4 }}>
          <StreamMarkdown content={controller.text} />
        </div>
      )}
    </div>
  );
}