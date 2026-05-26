import { XMarkdown } from '@ant-design/x-markdown';

interface StreamMarkdownProps {
  content: string;
  isStreaming?: boolean;
}

export function StreamMarkdown({ content }: StreamMarkdownProps) {
  return <XMarkdown content={content} />;
}