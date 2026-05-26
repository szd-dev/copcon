import type { CopConMessage } from '@copcon/chat-core';

interface MessageListOptions {
  autoScroll?: boolean;
}

interface MessageListController {
  shouldAutoScroll: (isAtBottom: boolean) => boolean;
  getItemProps: (msg: CopConMessage) => { key: string; role: string };
}

export function createMessageListController(
  options?: MessageListOptions,
): MessageListController {
  const autoScroll = options?.autoScroll ?? true;

  return {
    shouldAutoScroll: (isAtBottom: boolean) => autoScroll && isAtBottom,
    getItemProps: (msg: CopConMessage) => ({ key: msg.id, role: msg.role }),
  };
}
