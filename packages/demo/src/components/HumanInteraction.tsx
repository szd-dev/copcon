import { useMemo, useState } from 'react';
import { createToolCallController, createHitlFormController } from '@copcon/headless-hooks';
import type { ToolCallPart } from '@copcon/chat-core';
import { AgentClient } from '@copcon/chat-core';

interface HumanInteractionProps {
  part: ToolCallPart;
  sessionId: string;
  client: AgentClient;
}

export function HumanInteraction({ part, sessionId, client }: HumanInteractionProps) {
  const toolCallCtrl = useMemo(() => createToolCallController(part), [part]);
  const [loading, setLoading] = useState(false);
  const [formData, setFormData] = useState<Record<string, unknown>>({});

  if (!toolCallCtrl.needsApproval || !toolCallCtrl.interrupt) {
    return null;
  }

  const interrupt = toolCallCtrl.interrupt;

  const handleApprove = async () => {
    setLoading(true);
    try {
      await client.resume(sessionId, interrupt.interruptId, 'approve');
    } finally {
      setLoading(false);
    }
  };

  const handleDecline = async () => {
    setLoading(true);
    try {
      await client.resume(sessionId, interrupt.interruptId, 'decline');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    setLoading(true);
    try {
      await client.resume(sessionId, interrupt.interruptId, 'submit', formData);
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = async () => {
    setLoading(true);
    try {
      await client.resume(sessionId, interrupt.interruptId, 'cancel');
    } finally {
      setLoading(false);
    }
  };

  if (interrupt.interruptType === 'approval') {
    return (
      <div style={{ padding: 12, border: '1px solid #d9d9d9', borderRadius: 8, marginBottom: 8 }}>
        <div style={{ fontWeight: 500, marginBottom: 8 }}>{interrupt.message}</div>
        {interrupt.summary && (
          <div style={{ color: '#666', marginBottom: 8 }}>{interrupt.summary}</div>
        )}
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={handleApprove} disabled={loading} style={{ padding: '4px 16px', background: '#52c41a', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}>
            Approve
          </button>
          <button onClick={handleDecline} disabled={loading} style={{ padding: '4px 16px', background: '#ff4d4f', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}>
            Decline
          </button>
        </div>
      </div>
    );
  }

  // Question type — render form
  const hitlCtrl = createHitlFormController(interrupt, {
    onSubmit: () => {}, // handled by handleSubmit below
    onCancel: () => {},
  });

  const propertyNames = Object.keys(hitlCtrl.properties);

  return (
    <div style={{ padding: 12, border: '1px solid #d9d9d9', borderRadius: 8, marginBottom: 8 }}>
      <div style={{ fontWeight: 500, marginBottom: 8 }}>{interrupt.message}</div>
      <form onSubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
        {propertyNames.map((name) => {
          const fieldProps = hitlCtrl.getFormFieldProps(name);
          return (
            <div key={name} style={{ marginBottom: 8 }}>
              <label style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
                {fieldProps.label}{fieldProps.required ? ' *' : ''}
              </label>
              {fieldProps.type === 'select' || fieldProps.type === 'yesno' ? (
                <select
                  value={(formData[name] as string) ?? ''}
                  onChange={(e) => setFormData({ ...formData, [name]: e.target.value })}
                  style={{ width: '100%', padding: 4, borderRadius: 4, border: '1px solid #d9d9d9' }}
                >
                  <option value="">Select...</option>
                  {fieldProps.options?.map((opt) => (
                    <option key={opt} value={opt}>{opt}</option>
                  ))}
                </select>
              ) : fieldProps.type === 'number' ? (
                <input
                  type="number"
                  value={(formData[name] as string) ?? ''}
                  onChange={(e) => setFormData({ ...formData, [name]: Number(e.target.value) })}
                  style={{ width: '100%', padding: 4, borderRadius: 4, border: '1px solid #d9d9d9' }}
                />
              ) : (
                <input
                  type="text"
                  value={(formData[name] as string) ?? ''}
                  onChange={(e) => setFormData({ ...formData, [name]: e.target.value })}
                  style={{ width: '100%', padding: 4, borderRadius: 4, border: '1px solid #d9d9d9' }}
                />
              )}
            </div>
          );
        })}
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <button type="submit" disabled={loading} style={{ padding: '4px 16px', background: '#1677ff', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}>
            Submit
          </button>
          <button type="button" onClick={handleCancel} disabled={loading} style={{ padding: '4px 16px', border: '1px solid #d9d9d9', borderRadius: 4, cursor: 'pointer' }}>
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}