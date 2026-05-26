import type { InterruptPayload } from '@copcon/chat-core';

interface HitlFormCallbacks {
  onSubmit: (formData: Record<string, unknown>) => void;
  onCancel: () => void;
}

interface FormFieldProps {
  type: 'text' | 'number' | 'select' | 'yesno';
  label: string;
  options?: string[];
  required: boolean;
}

interface HitlFormController {
  schema: Record<string, unknown> | undefined;
  required: string[];
  properties: Record<string, unknown>;
  getFormFieldProps: (name: string) => FormFieldProps;
  handleSubmit: (formData: Record<string, unknown>) => void;
  handleCancel: () => void;
  getContainerProps: () => { role: string; 'aria-label': string };
}

export function createHitlFormController(
  interrupt: InterruptPayload,
  callbacks: HitlFormCallbacks,
): HitlFormController {
  const schema = interrupt.inputSchema;
  const properties = (schema?.properties as Record<string, unknown>) ?? {};
  const required = (schema?.required as string[]) ?? [];

  return {
    schema,
    required,
    properties,
    getFormFieldProps: (name: string): FormFieldProps => {
      const fieldDef = properties[name] as Record<string, unknown> | undefined;
      if (!fieldDef) {
        return { type: 'text', label: name, required: required.includes(name) };
      }

      if (Array.isArray(fieldDef.enum)) {
        return {
          type: 'select',
          label: name,
          options: fieldDef.enum as string[],
          required: required.includes(name),
        };
      }

      if (fieldDef.type === 'number' || fieldDef.type === 'integer') {
        return { type: 'number', label: name, required: required.includes(name) };
      }

      if (fieldDef.type === 'boolean') {
        return { type: 'yesno', label: name, options: ['Yes', 'No'], required: required.includes(name) };
      }

      return { type: 'text', label: name, required: required.includes(name) };
    },
    handleSubmit: (formData: Record<string, unknown>) => callbacks.onSubmit(formData),
    handleCancel: () => callbacks.onCancel(),
    getContainerProps: () => ({ role: 'form', 'aria-label': interrupt.message }),
  };
}
