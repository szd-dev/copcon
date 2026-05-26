import type { ReasoningPart } from '@copcon/chat-core';

interface ThinkingChainOptions {
  defaultExpanded?: boolean;
  autoCollapse?: boolean;
}

interface ThinkingChainController {
  expanded: boolean;
  isStreaming: boolean;
  text: string;
  toggle: () => void;
  getContainerProps: () => { role: string; 'aria-label': string };
  getToggleProps: () => { 'aria-expanded': boolean };
  getContentProps: () => { hidden: boolean };
}

export function createThinkingChainController(
  part: ReasoningPart,
  options?: ThinkingChainOptions,
): ThinkingChainController {
  let expanded = options?.defaultExpanded ?? false;

  return {
    expanded,
    isStreaming: part.state === 'streaming',
    text: part.text,
    toggle: () => { expanded = !expanded; },
    getContainerProps: () => ({ role: 'region', 'aria-label': 'Thinking' }),
    getToggleProps: () => ({ 'aria-expanded': expanded }),
    getContentProps: () => ({ hidden: !expanded }),
  };
}