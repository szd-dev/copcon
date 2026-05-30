import React from 'react';
import { Modal, Form, Input, Button } from 'antd';

interface CreateKBModalProps {
  open: boolean;
  onCancel: () => void;
  onSubmit: (values: { name: string }) => void;
  loading?: boolean;
}

export const CreateKBModal: React.FC<CreateKBModalProps> = ({
  open,
  onCancel,
  onSubmit,
  loading,
}) => {
  const [form] = Form.useForm();

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      onSubmit(values);
      form.resetFields();
    } catch {
      return;
    }
  };

  return (
    <Modal
      title="Create Knowledge Base"
      open={open}
      onCancel={onCancel}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          Cancel
        </Button>,
        <Button key="submit" type="primary" loading={loading} onClick={handleSubmit}>
          Create
        </Button>,
      ]}
      destroyOnClose
    >
      <Form form={form} layout="vertical" autoComplete="off">
        <Form.Item
          label="Name"
          name="name"
          rules={[
            { required: true, message: 'Please enter a name' },
            { min: 1, message: 'Name cannot be empty' },
            { max: 100, message: 'Name too long' },
          ]}
        >
          <Input placeholder="Enter knowledge base name" autoFocus />
        </Form.Item>
      </Form>
    </Modal>
  );
};
