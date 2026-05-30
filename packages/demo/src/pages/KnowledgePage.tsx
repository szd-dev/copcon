import React, { useState, useEffect, useCallback } from 'react';
import { Flex, message, theme } from 'antd';
import { useClient } from '../context/ClientContext';
import { KBList } from '../components/kb/KBList';
import { KBDetail } from '../components/kb/KBDetail';
import { CreateKBModal } from '../components/kb/CreateKBModal';
import { KBUpload } from '../components/kb/KBUpload';
import { ChunkViewer } from '../components/kb/ChunkViewer';
import { KBRetrievalTest } from '../components/kb/KBRetrievalTest';
import type { KnowledgeBase, Document } from '@copcon/chat-core';

const { useToken } = theme;

const KnowledgePage: React.FC = () => {
  const { token } = useToken();
  const client = useClient();
  const [kbs, setKbs] = useState<KnowledgeBase[]>([]);
  const [loadingKbs, setLoadingKbs] = useState(true);
  const [selectedKB, setSelectedKB] = useState<KnowledgeBase | undefined>();
  const [documents, setDocuments] = useState<Document[]>([]);
  const [loadingDocs, setLoadingDocs] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [chunkViewerOpen, setChunkViewerOpen] = useState(false);
  const [selectedDocId, setSelectedDocId] = useState('');
  const [retrievalOpen, setRetrievalOpen] = useState(false);

  const loadKbs = useCallback(async () => {
    setLoadingKbs(true);
    try {
      const result = await client.listKnowledgeBases();
      setKbs(result.knowledge_bases || []);
    } catch {
      message.error('Failed to load knowledge bases');
    } finally {
      setLoadingKbs(false);
    }
  }, [client]);

  const loadDocuments = useCallback(
    async (kbId: string) => {
      setLoadingDocs(true);
      try {
        const result = await client.listDocuments(kbId);
        setDocuments(result.documents || []);
      } catch {
        message.error('Failed to load documents');
        setDocuments([]);
      } finally {
        setLoadingDocs(false);
      }
    },
    [client]
  );

  useEffect(() => {
    loadKbs();
  }, [loadKbs]);

  useEffect(() => {
    if (selectedKB) {
      loadDocuments(selectedKB.id);
    } else {
      setDocuments([]);
    }
  }, [selectedKB, loadDocuments]);

  const handleCreate = async (values: { name: string }) => {
    setCreating(true);
    try {
      const kb = await client.createKnowledgeBase(values.name);
      setKbs((prev) => [kb, ...prev]);
      setSelectedKB(kb);
      setCreateModalOpen(false);
      message.success('Knowledge base created');
    } catch {
      message.error('Failed to create knowledge base');
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteKB = async (kbId: string) => {
    try {
      await client.deleteKnowledgeBase(kbId);
      setKbs((prev) => prev.filter((k) => k.id !== kbId));
      if (selectedKB?.id === kbId) {
        setSelectedKB(undefined);
      }
      message.success('Knowledge base deleted');
    } catch {
      message.error('Failed to delete knowledge base');
    }
  };

  const handleDeleteDocument = async (docId: string) => {
    if (!selectedKB) return;
    try {
      await client.deleteDocument(selectedKB.id, docId);
      setDocuments((prev) => prev.filter((d) => d.id !== docId));
      message.success('Document deleted');
    } catch {
      message.error('Failed to delete document');
    }
  };

  const handleUploadSuccess = () => {
    setUploadOpen(false);
    if (selectedKB) {
      loadDocuments(selectedKB.id);
    }
    message.success('Documents uploaded');
  };

  const handleViewChunks = (docId: string) => {
    setSelectedDocId(docId);
    setChunkViewerOpen(true);
  };

  return (
    <Flex style={{ height: '100%' }}>
      <Flex
        vertical
        style={{
          width: 320,
          borderRight: `1px solid ${token.colorBorderSecondary}`,
          overflow: 'auto',
        }}
      >
        <KBList
          knowledgeBases={kbs}
          loading={loadingKbs}
          selectedId={selectedKB?.id}
          onSelect={setSelectedKB}
          onDelete={handleDeleteKB}
          onCreate={() => setCreateModalOpen(true)}
        />
      </Flex>

      <Flex flex={1} style={{ overflow: 'auto' }}>
        <KBDetail
          knowledgeBase={selectedKB}
          documents={documents}
          loading={loadingDocs}
          onUpload={() => setUploadOpen(true)}
          onViewChunks={handleViewChunks}
          onDeleteDocument={handleDeleteDocument}
          onTestRetrieval={() => setRetrievalOpen(true)}
        />
      </Flex>

      <CreateKBModal
        open={createModalOpen}
        onCancel={() => setCreateModalOpen(false)}
        onSubmit={handleCreate}
        loading={creating}
      />

      {selectedKB && (
        <>
          <KBUpload
            open={uploadOpen}
            kbId={selectedKB.id}
            onClose={() => setUploadOpen(false)}
            onSuccess={handleUploadSuccess}
          />
          <ChunkViewer
            open={chunkViewerOpen}
            kbId={selectedKB.id}
            docId={selectedDocId}
            onClose={() => setChunkViewerOpen(false)}
          />
          <KBRetrievalTest
            open={retrievalOpen}
            kbId={selectedKB.id}
            onClose={() => setRetrievalOpen(false)}
          />
        </>
      )}
    </Flex>
  );
};

export default KnowledgePage;
