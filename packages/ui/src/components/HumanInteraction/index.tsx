import React, { useState } from 'react';
import { Button, Card, Typography, Space, Form, Input, InputNumber, Select } from 'antd';
import { CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import type { InterruptPayload } from '../../api/types';

export interface HumanInteractionProps {
  interrupt: InterruptPayload;
  onRespond: (action: 'approve' | 'decline' | 'submit' | 'cancel', content?: Record<string, unknown>) => void;
  loading?: boolean;
}

export const HumanInteraction: React.FC<HumanInteractionProps> = ({ interrupt, onRespond, loading }) => {
  const [formData, setFormData] = useState<Record<string, unknown>>({});

  if (interrupt.interruptType === 'approval') {
    return (
      <Card size="small" style={{ marginTop: 8 }}>
        <Typography.Text>{interrupt.message}</Typography.Text>
        {interrupt.summary && (
          <Typography.Paragraph type="secondary" style={{ margin: '8px 0' }}>
            {interrupt.summary}
          </Typography.Paragraph>
        )}
        <Space style={{ marginTop: 8 }}>
          <Button
            type="primary"
            icon={<CheckCircleOutlined />}
            loading={loading}
            onClick={() => onRespond('approve')}
          >
            Approve
          </Button>
          <Button
            danger
            icon={<CloseCircleOutlined />}
            loading={loading}
            onClick={() => onRespond('decline')}
          >
            Decline
          </Button>
        </Space>
      </Card>
    );
  }

  const schema = interrupt.inputSchema;
  const properties = (schema?.properties || {}) as Record<string, { type: string; title?: string; enum?: string[]; optionLabels?: Record<string, string> }>;
  const required = (schema?.required || []) as string[];

  const renderField = (name: string, fieldSchema: { type: string; title?: string; enum?: string[]; optionLabels?: Record<string, string> }) => {
    const label = fieldSchema.title || name;
    const isRequired = required.includes(name);

    if (fieldSchema.enum) {
      const optLabels = fieldSchema.optionLabels || {};
      return (
        <Form.Item key={name} label={label} required={isRequired}>
          <Select
            options={fieldSchema.enum.map((v) => ({ label: optLabels[v] || v, value: v }))}
            onChange={(v) => setFormData((prev) => ({ ...prev, [name]: v }))}
          />
        </Form.Item>
      );
    }

    switch (fieldSchema.type) {
      case 'number':
        return (
          <Form.Item key={name} label={label} required={isRequired}>
            <InputNumber onChange={(v) => setFormData((prev) => ({ ...prev, [name]: v }))} style={{ width: '100%' }} />
          </Form.Item>
        );
      case 'boolean':
        return (
          <Form.Item key={name} label={label} required={isRequired}>
            <Select
              options={[{ label: 'Yes', value: true }, { label: 'No', value: false }]}
              onChange={(v) => setFormData((prev) => ({ ...prev, [name]: v }))}
            />
          </Form.Item>
        );
      default:
        return (
          <Form.Item key={name} label={label} required={isRequired}>
            <Input onChange={(e) => setFormData((prev) => ({ ...prev, [name]: e.target.value }))} />
          </Form.Item>
        );
    }
  };

  return (
    <Card size="small" style={{ marginTop: 8 }}>
      <Typography.Text>{interrupt.message}</Typography.Text>
      <Form layout="vertical" style={{ marginTop: 8 }}>
        {Object.entries(properties).map(([name, fieldSchema]) => renderField(name, fieldSchema))}
      </Form>
      <Space style={{ marginTop: 8 }}>
        <Button
          type="primary"
          icon={<CheckCircleOutlined />}
          loading={loading}
          onClick={() => onRespond('submit', formData)}
        >
          Submit
        </Button>
        <Button
          icon={<CloseCircleOutlined />}
          loading={loading}
          onClick={() => onRespond('cancel')}
        >
          Cancel
        </Button>
      </Space>
    </Card>
  );
};

export default HumanInteraction;
