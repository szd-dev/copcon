import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  Tag,
  Empty,
  Spin,
  Button,
  Popconfirm,
  message,
  Typography,
  Flex,
  theme,
} from 'antd';
import { useClient } from '../context/ClientContext';
import type { SkillInfo, SkillDetail } from '@copcon/chat-core';

const { useToken } = theme;
const { Text, Title } = Typography;

const SkillPage: React.FC = () => {
  const { token } = useToken();
  const client = useClient();
  const [skills, setSkills] = useState<SkillInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedSkill, setSelectedSkill] = useState<SkillDetail | undefined>();
  const [loadingDetail, setLoadingDetail] = useState(false);

  const loadSkills = useCallback(async () => {
    setLoading(true);
    try {
      const result = await client.listSkills();
      setSkills(result.skills || []);
    } catch {
      message.error('Failed to load skills');
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    loadSkills();
  }, [loadSkills]);

  const selectSkill = useCallback(
    async (skill: SkillInfo) => {
      setLoadingDetail(true);
      try {
        const detail = await client.getSkill(skill.name, true);
        setSelectedSkill(detail);
      } catch {
        message.error('Failed to load skill details');
      } finally {
        setLoadingDetail(false);
      }
    },
    [client]
  );

  const toggleSkill = useCallback(
    async (name: string, enable: boolean) => {
      try {
        if (enable) {
          await client.enableSkill(name);
        } else {
          await client.disableSkill(name);
        }
        message.success(`Skill ${enable ? 'enabled' : 'disabled'}`);
        loadSkills();
        if (selectedSkill?.name === name) {
          const detail = await client.getSkill(name, true);
          setSelectedSkill(detail);
        }
      } catch {
        message.error(`Failed to ${enable ? 'enable' : 'disable'} skill`);
      }
    },
    [client, loadSkills, selectedSkill]
  );

  return (
    <Flex style={{ height: '100%' }}>
      <div
        style={{
          width: 320,
          borderRight: `1px solid ${token.colorBorderSecondary}`,
          overflow: 'auto',
          padding: token.padding,
        }}
      >
        {loading ? (
          <Flex justify="center" align="center" style={{ height: 200 }}>
            <Spin />
          </Flex>
        ) : skills.length === 0 ? (
          <Empty description="No skills found" />
        ) : (
          <Flex vertical gap={8}>
            {skills.map((skill) => (
              <Card
                key={skill.name}
                size="small"
                hoverable
                onClick={() => selectSkill(skill)}
                style={{
                  borderColor:
                    selectedSkill?.name === skill.name
                      ? token.colorPrimary
                      : token.colorBorderSecondary,
                  borderWidth: selectedSkill?.name === skill.name ? 2 : 1,
                  cursor: 'pointer',
                }}
              >
                <Flex vertical gap={4}>
                  <Flex justify="space-between" align="center">
                    <Text strong>{skill.name}</Text>
                    <Tag color={skill.enabled ? 'green' : 'default'}>
                      {skill.enabled ? 'Enabled' : 'Disabled'}
                    </Tag>
                  </Flex>
                  <Text type="secondary" ellipsis>
                    {skill.description}
                  </Text>
                </Flex>
              </Card>
            ))}
          </Flex>
        )}
      </div>

      <div
        style={{
          flex: 1,
          overflow: 'auto',
          padding: token.paddingLG,
        }}
      >
        {!selectedSkill ? (
          <Flex justify="center" align="center" style={{ height: '100%' }}>
            <Empty description="Select a skill to view details" />
          </Flex>
        ) : loadingDetail ? (
          <Flex justify="center" align="center" style={{ height: '100%' }}>
            <Spin />
          </Flex>
        ) : (
          <Flex vertical gap={16}>
            <Flex justify="space-between" align="center">
              <Title level={3} style={{ margin: 0 }}>
                {selectedSkill.name}
              </Title>
              <Popconfirm
                title={
                  selectedSkill.enabled
                    ? `Disable skill "${selectedSkill.name}"?`
                    : `Enable skill "${selectedSkill.name}"?`
                }
                onConfirm={() =>
                  toggleSkill(selectedSkill.name, !selectedSkill.enabled)
                }
                okText="Confirm"
                cancelText="Cancel"
              >
                <Button
                  type={selectedSkill.enabled ? 'default' : 'primary'}
                  danger={selectedSkill.enabled}
                >
                  {selectedSkill.enabled ? 'Disable' : 'Enable'}
                </Button>
              </Popconfirm>
            </Flex>

            <Text type="secondary">{selectedSkill.description}</Text>

            {selectedSkill.source && (
              <Text type="secondary">Source: {selectedSkill.source}</Text>
            )}

            {selectedSkill.metadata &&
              Object.keys(selectedSkill.metadata).length > 0 && (
                <Card size="small" title="Metadata">
                  {Object.entries(selectedSkill.metadata).map(([k, v]) => (
                    <div key={k}>
                      <Text strong>{k}:</Text> <Text>{v}</Text>
                    </div>
                  ))}
                </Card>
              )}

            {selectedSkill.instructions && (
              <Card size="small" title="Instructions">
                <pre
                  style={{
                    whiteSpace: 'pre-wrap',
                    fontFamily: 'monospace',
                    fontSize: 13,
                    margin: 0,
                  }}
                >
                  {selectedSkill.instructions}
                </pre>
              </Card>
            )}

            {selectedSkill.resource_files &&
              selectedSkill.resource_files.length > 0 && (
                <Card size="small" title="Resource Files">
                  {selectedSkill.resource_files.map((rf, i) => (
                    <div key={i}>
                      <Tag>{rf.category}</Tag> <Text>{rf.name}</Text>
                    </div>
                  ))}
                </Card>
              )}

            {selectedSkill.allowed_tools && (
              <Card size="small" title="Allowed Tools">
                <Text code>{selectedSkill.allowed_tools}</Text>
              </Card>
            )}
          </Flex>
        )}
      </div>
    </Flex>
  );
};

export default SkillPage;
