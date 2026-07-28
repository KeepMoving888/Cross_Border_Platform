/**
 * 知识库管理(RAG):左侧知识库列表 + 右侧文档列表 / 检索测试 双 Tab
 * - 左侧:知识库卡片,支持创建,显示文档数/向量维度/Embedding 模型
 * - 右侧 Tab1 文档管理:上传文档(纯文本)、状态轮询(processing→ready/failed)、分块数
 * - 右侧 Tab2 检索测试:输入 query + topK,实时展示召回分块+相似度,验证 RAG 效果
 * 对接后端:
 *   GET  /api/v1/ai/knowledge-bases
 *   POST /api/v1/ai/knowledge-bases
 *   GET  /api/v1/ai/knowledge-bases/:id/documents
 *   POST /api/v1/ai/knowledge-bases/:id/documents
 *   POST /api/v1/ai/rag/search
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Card,
  Row,
  Col,
  Button,
  Tag,
  Table,
  Space,
  Typography,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  Spin,
  Empty,
  Descriptions,
  message,
  Statistic,
  Progress,
  Tooltip,
  List,
  Divider,
  Alert,
} from 'antd';
import {
  DatabaseOutlined,
  FileTextOutlined,
  PlusOutlined,
  ReloadOutlined,
  UploadOutlined,
  SearchOutlined,
  CheckCircleFilled,
  CloseCircleFilled,
  LoadingOutlined,
  ThunderboltOutlined,
  CopyOutlined,
} from '@ant-design/icons';
import PageContainer from '@/components/PageContainer';
import {
  listKnowledgeBases,
  createKnowledgeBase,
  listDocuments,
  uploadDocument,
  ragSearch,
} from '@/api/ai';
import { formatDateTime } from '@/utils/format';
import type { KnowledgeBase, KnowledgeDocument, RAGDocument } from '@/types/api';

const { Text, Paragraph } = Typography;

/** 知识库类型字典 */
const KB_TYPE_MAP: Record<string, { label: string; color: string }> = {
  product_manual: { label: '产品手册', color: 'blue' },
  purchase_contract: { label: '采购合同', color: 'purple' },
  faq: { label: '常见问答', color: 'cyan' },
  operation_guide: { label: '运营指南', color: 'magenta' },
  '': { label: '通用', color: 'default' },
};

/** 文档状态字典(processing=蓝脉冲/ready=绿/failed=红) */
const DOC_STATUS_MAP: Record<string, { label: string; color: string; icon: React.ReactNode }> = {
  processing: {
    label: '处理中',
    color: 'processing',
    icon: <LoadingOutlined style={{ color: '#1677ff' }} />,
  },
  ready: {
    label: '已就绪',
    color: 'success',
    icon: <CheckCircleFilled style={{ color: '#52c41a' }} />,
  },
  failed: {
    label: '失败',
    color: 'error',
    icon: <CloseCircleFilled style={{ color: '#ff4d4f' }} />,
  },
};

const KnowledgeBases: React.FC = () => {
  // 知识库列表
  const [kbList, setKbList] = useState<KnowledgeBase[]>([]);
  const [kbLoading, setKbLoading] = useState(false);
  const [selectedKb, setSelectedKb] = useState<KnowledgeBase | null>(null);

  // 创建知识库 Modal
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [createForm] = Form.useForm();

  // 上传文档 Modal
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [uploadSubmitting, setUploadSubmitting] = useState(false);
  const [uploadForm] = Form.useForm();

  // 文档列表
  const [docs, setDocs] = useState<KnowledgeDocument[]>([]);
  const [docsLoading, setDocsLoading] = useState(false);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // 检索测试
  const [searchQuery, setSearchQuery] = useState('');
  const [searchTopK, setSearchTopK] = useState(5);
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState<RAGDocument[]>([]);

  // 当前激活 Tab
  const [activeTab, setActiveTab] = useState<'documents' | 'search'>('documents');

  // ============== 数据加载 ==============

  const refreshKbList = useCallback(async () => {
    setKbLoading(true);
    try {
      const list = await listKnowledgeBases();
      setKbList(list || []);
      // 自动选中第一个(若未选中)
      if (!selectedKb && list && list.length > 0) {
        setSelectedKb(list[0]);
      }
    } catch (e) {
      // client.ts 已统一报错,这里静默
    } finally {
      setKbLoading(false);
    }
  }, [selectedKb]);

  const refreshDocs = useCallback(async (kbID: number) => {
    setDocsLoading(true);
    try {
      const list = await listDocuments(kbID);
      setDocs(list || []);
    } catch (e) {
      // 静默
    } finally {
      setDocsLoading(false);
    }
  }, []);

  // 初次加载
  useEffect(() => {
    refreshKbList();
  }, [refreshKbList]);

  // 选中知识库后加载文档
  useEffect(() => {
    if (selectedKb) {
      refreshDocs(selectedKb.id);
    } else {
      setDocs([]);
    }
  }, [selectedKb, refreshDocs]);

  // 文档状态轮询:存在 processing 状态时每 3s 刷新
  useEffect(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (!selectedKb) return;
    const hasProcessing = docs.some((d) => d.status === 'processing');
    if (!hasProcessing) return;
    pollTimerRef.current = setInterval(() => {
      refreshDocs(selectedKb.id);
    }, 3000);
    return () => {
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, [docs, selectedKb, refreshDocs]);

  // ============== 创建知识库 ==============

  const handleCreateKb = async () => {
    try {
      const values = await createForm.validateFields();
      setCreateSubmitting(true);
      await createKnowledgeBase({
        name: values.name,
        code: values.code,
        description: values.description,
        type: values.type,
      });
      message.success('知识库创建成功');
      setCreateModalOpen(false);
      createForm.resetFields();
      await refreshKbList();
    } catch (e) {
      // 校验或 API 错误
    } finally {
      setCreateSubmitting(false);
    }
  };

  // ============== 上传文档 ==============

  const handleUploadDoc = async () => {
    if (!selectedKb) return;
    try {
      const values = await uploadForm.validateFields();
      setUploadSubmitting(true);
      await uploadDocument(selectedKb.id, {
        title: values.title,
        content: values.content,
        source: values.source,
      });
      message.success('文档上传成功,正在后台分块向量化');
      setUploadModalOpen(false);
      uploadForm.resetFields();
      await refreshDocs(selectedKb.id);
      // 知识库文档计数 +1(本地乐观更新)
      setSelectedKb({ ...selectedKb, document_count: selectedKb.document_count + 1 });
    } catch (e) {
      // 静默
    } finally {
      setUploadSubmitting(false);
    }
  };

  // ============== RAG 检索测试 ==============

  const handleSearch = async () => {
    if (!selectedKb) {
      message.warning('请先选择知识库');
      return;
    }
    if (!searchQuery.trim()) {
      message.warning('请输入检索问题');
      return;
    }
    setSearching(true);
    try {
      const res = await ragSearch({
        query: searchQuery.trim(),
        knowledge_base_id: selectedKb.id,
        top_k: searchTopK,
      });
      setSearchResults(res?.documents || []);
      if (!res?.documents || res.documents.length === 0) {
        message.info('未召回任何相关文档,可尝试调整 query 或检查知识库内容');
      }
    } catch (e) {
      // 静默
    } finally {
      setSearching(false);
    }
  };

  // ============== 渲染辅助 ==============

  /** 文档表格列定义 */
  const docColumns = useMemo(
    () => [
      {
        title: '文档标题',
        dataIndex: 'title',
        key: 'title',
        ellipsis: true,
        width: 220,
        render: (text: string) => <Text strong>{text}</Text>,
      },
      {
        title: '来源',
        dataIndex: 'source',
        key: 'source',
        ellipsis: true,
        width: 180,
        render: (text: string) => (text ? <Text type="secondary">{text}</Text> : <Text type="secondary">-</Text>),
      },
      {
        title: '分块数',
        dataIndex: 'chunk_count',
        key: 'chunk_count',
        width: 90,
        align: 'center' as const,
        render: (n: number) => <Tag color="blue">{n || 0}</Tag>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 120,
        render: (status: string, record: KnowledgeDocument) => {
          const map = DOC_STATUS_MAP[status] || DOC_STATUS_MAP.processing;
          return (
            <Space size={4}>
              {map.icon}
              <Tag color={map.color}>{map.label}</Tag>
              {status === 'failed' && record.error && (
                <Tooltip title={record.error}>
                  <Text type="danger" style={{ fontSize: 12 }}>
                    详情
                  </Text>
                </Tooltip>
              )}
            </Space>
          );
        },
      },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        key: 'created_at',
        width: 160,
        render: (t: string) => <Text type="secondary">{formatDateTime(t)}</Text>,
      },
    ],
    [],
  );

  /** 检索结果相似度进度条颜色 */
  const scoreColor = (score: number) => {
    if (score >= 0.8) return '#52c41a';
    if (score >= 0.6) return '#faad14';
    if (score >= 0.4) return '#fa8c16';
    return '#ff4d4f';
  };

  // ============== UI ==============

  return (
    <PageContainer
      title="知识库管理"
      subTitle="RAG 向量检索知识库 · 文档分块 + Embedding 入库 + 语义检索"
      extra={[
        <Button
          key="refresh"
          icon={<ReloadOutlined />}
          onClick={() => {
            refreshKbList();
            if (selectedKb) refreshDocs(selectedKb.id);
          }}
        >
          刷新
        </Button>,
        <Button
          key="create"
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setCreateModalOpen(true)}
        >
          新建知识库
        </Button>,
      ]}
    >
      <Row gutter={16}>
        {/* ============== 左侧:知识库列表 ============== */}
        <Col xs={24} md={7} lg={6}>
          <Card
            title={
              <Space>
                <DatabaseOutlined />
                <span>知识库</span>
              </Space>
            }
            bodyStyle={{ padding: 0 }}
            loading={kbLoading}
          >
            <List
              dataSource={kbList}
              locale={{
                emptyText: (
                  <Empty
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                    description="暂无知识库"
                    style={{ padding: '32px 0' }}
                  >
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
                      新建知识库
                    </Button>
                  </Empty>
                ),
              }}
              renderItem={(kb) => {
                const active = selectedKb?.id === kb.id;
                const typeInfo = KB_TYPE_MAP[kb.type] || KB_TYPE_MAP[''];
                return (
                  <List.Item
                    onClick={() => setSelectedKb(kb)}
                    style={{
                      cursor: 'pointer',
                      padding: '12px 16px',
                      background: active ? '#e6f4ff' : 'transparent',
                      borderLeft: active ? '3px solid #1677ff' : '3px solid transparent',
                      transition: 'all 0.2s',
                    }}
                  >
                    <div style={{ width: '100%' }}>
                      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                        <Text strong ellipsis style={{ maxWidth: 160 }}>
                          {kb.name}
                        </Text>
                        <Tag color={typeInfo.color}>{typeInfo.label}</Tag>
                      </Space>
                      <div style={{ marginTop: 4 }}>
                        <Space size="small" style={{ color: '#8c8c8c', fontSize: 12 }}>
                          <FileTextOutlined />
                          <span>{kb.document_count || 0} 文档</span>
                          <Divider type="vertical" style={{ margin: '0 4px' }} />
                          <span>{kb.dimension || 1536} 维</span>
                        </Space>
                      </div>
                      {kb.description && (
                        <Paragraph
                          type="secondary"
                          ellipsis={{ rows: 1 }}
                          style={{ margin: '4px 0 0', fontSize: 12 }}
                        >
                          {kb.description}
                        </Paragraph>
                      )}
                    </div>
                  </List.Item>
                );
              }}
            />
          </Card>
        </Col>

        {/* ============== 右侧:文档管理 / 检索测试 ============== */}
        <Col xs={24} md={17} lg={18}>
          {!selectedKb ? (
            <Card>
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="请从左侧选择一个知识库"
                style={{ padding: '60px 0' }}
              />
            </Card>
          ) : (
            <Card
              title={
                <Space>
                  <span>{selectedKb.name}</span>
                  <Tag color={KB_TYPE_MAP[selectedKb.type]?.color || 'default'}>
                    {KB_TYPE_MAP[selectedKb.type]?.label || '通用'}
                  </Tag>
                  <Tag color={selectedKb.status === 'enabled' ? 'success' : 'default'}>
                    {selectedKb.status === 'enabled' ? '启用' : '禁用'}
                  </Tag>
                </Space>
              }
              extra={
                <Space>
                  <Statistic
                    title="文档总数"
                    value={selectedKb.document_count || 0}
                    prefix={<FileTextOutlined />}
                    valueStyle={{ fontSize: 16 }}
                  />
                </Space>
              }
              tabList={[
                { key: 'documents', tab: '文档管理' },
                { key: 'search', tab: '检索测试' },
              ]}
              tabProps={{ size: 'middle' }}
              activeTabKey={activeTab}
              onTabChange={(k) => setActiveTab(k as 'documents' | 'search')}
            >
              {/* 知识库元信息 */}
              <Descriptions size="small" column={3} style={{ marginBottom: 16 }}>
                <Descriptions.Item label="Embedding 模型">
                  <Text code>{selectedKb.embedding_model || 'text-embedding-ada-002'}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="向量维度">{selectedKb.dimension || 1536}</Descriptions.Item>
                <Descriptions.Item label="创建时间">
                  {formatDateTime(selectedKb.created_at)}
                </Descriptions.Item>
              </Descriptions>

              {activeTab === 'documents' && (
                <Spin spinning={docsLoading}>
                  <div style={{ marginBottom: 12 }}>
                    <Space>
                      <Button
                        type="primary"
                        icon={<UploadOutlined />}
                        onClick={() => setUploadModalOpen(true)}
                      >
                        上传文档
                      </Button>
                      <Button icon={<ReloadOutlined />} onClick={() => refreshDocs(selectedKb.id)}>
                        刷新
                      </Button>
                    </Space>
                  </div>
                  <Table
                    rowKey="id"
                    columns={docColumns}
                    dataSource={docs}
                    pagination={{ pageSize: 10, showSizeChanger: true }}
                    size="middle"
                    locale={{
                      emptyText: (
                        <Empty
                          image={Empty.PRESENTED_IMAGE_SIMPLE}
                          description="暂无文档,点击「上传文档」开始"
                        />
                      ),
                    }}
                  />
                </Spin>
              )}

              {activeTab === 'search' && (
                <div>
                  <Alert
                    message="RAG 检索测试"
                    description="输入自然语言问题,验证向量检索召回质量。Top-1 相似度 > 0.7 表示检索效果良好;< 0.4 建议优化分块策略或补充知识库内容。"
                    type="info"
                    showIcon
                    style={{ marginBottom: 16 }}
                  />
                  <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
                    <Input
                      placeholder="输入检索问题,如:吹风机的负离子功能如何工作?"
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      onPressEnter={handleSearch}
                      prefix={<SearchOutlined />}
                      style={{ flex: 1 }}
                    />
                    <InputNumber
                      min={1}
                      max={20}
                      value={searchTopK}
                      onChange={(v) => setSearchTopK(Number(v) || 5)}
                      style={{ width: 110 }}
                      addonBefore="TopK"
                    />
                    <Button
                      type="primary"
                      icon={<ThunderboltOutlined />}
                      loading={searching}
                      onClick={handleSearch}
                    >
                      检索
                    </Button>
                  </Space.Compact>

                  {searchResults.length === 0 && !searching && (
                    <Empty
                      image={Empty.PRESENTED_IMAGE_SIMPLE}
                      description="输入问题后点击「检索」查看召回结果"
                      style={{ padding: '40px 0' }}
                    />
                  )}

                  {searchResults.length > 0 && (
                    <div>
                      <div style={{ marginBottom: 8 }}>
                        <Text type="secondary">
                          召回 <Text strong>{searchResults.length}</Text> 条相关文档分块
                        </Text>
                      </div>
                      <List
                        dataSource={searchResults}
                        renderItem={(doc, idx) => (
                          <List.Item style={{ alignItems: 'flex-start', padding: '12px 0' }}>
                            <Card
                              size="small"
                              style={{ width: '100%' }}
                              title={
                                <Space>
                                  <Tag color="blue">#{idx + 1}</Tag>
                                  <Text strong>{doc.title || `分块 ${doc.chunk_idx + 1}`}</Text>
                                  {doc.source && (
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                      来源:{doc.source}
                                    </Text>
                                  )}
                                </Space>
                              }
                              extra={
                                <Tooltip title={`余弦相似度: ${(doc.score * 100).toFixed(1)}%`}>
                                  <div style={{ width: 120, textAlign: 'right' }}>
                                    <Progress
                                      percent={Math.round(doc.score * 100)}
                                      size="small"
                                      strokeColor={scoreColor(doc.score)}
                                      format={(p) => (
                                        <span style={{ color: scoreColor(doc.score), fontWeight: 600 }}>
                                          {p}%
                                        </span>
                                      )}
                                    />
                                  </div>
                                </Tooltip>
                              }
                            >
                              <Paragraph
                                style={{ margin: 0, whiteSpace: 'pre-wrap' }}
                                ellipsis={{ rows: 4, expandable: true, symbol: '展开' }}
                              >
                                {doc.content}
                              </Paragraph>
                              <Space
                                size="small"
                                style={{ marginTop: 8, color: '#8c8c8c', fontSize: 12 }}
                              >
                                <span>分块索引: {doc.chunk_idx}</span>
                                <Divider type="vertical" />
                                <span>字符数: {doc.content.length}</span>
                                <Divider type="vertical" />
                                <Button
                                  type="link"
                                  size="small"
                                  icon={<CopyOutlined />}
                                  onClick={() => {
                                    navigator.clipboard.writeText(doc.content);
                                    message.success('已复制到剪贴板');
                                  }}
                                >
                                  复制
                                </Button>
                              </Space>
                            </Card>
                          </List.Item>
                        )}
                      />
                    </div>
                  )}
                </div>
              )}
            </Card>
          )}
        </Col>
      </Row>

      {/* ============== 创建知识库 Modal ============== */}
      <Modal
        title="新建知识库"
        open={createModalOpen}
        onCancel={() => setCreateModalOpen(false)}
        onOk={handleCreateKb}
        confirmLoading={createSubmitting}
        okText="创建"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={createForm} layout="vertical" preserve={false}>
          <Form.Item
            name="name"
            label="知识库名称"
            rules={[{ required: true, message: '请输入知识库名称' }]}
          >
            <Input placeholder="如:产品说明书知识库" maxLength={64} />
          </Form.Item>
          <Form.Item name="code" label="知识库代码" tooltip="可选,用于程序引用,唯一">
            <Input placeholder="如:kb_product_manual" maxLength={32} />
          </Form.Item>
          <Form.Item name="type" label="知识库类型" initialValue="">
            <Select
              options={[
                { value: '', label: '通用' },
                { value: 'product_manual', label: '产品手册' },
                { value: 'purchase_contract', label: '采购合同' },
                { value: 'faq', label: '常见问答' },
                { value: 'operation_guide', label: '运营指南' },
              ]}
            />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea placeholder="知识库用途说明,便于后续维护" rows={3} maxLength={256} />
          </Form.Item>
        </Form>
      </Modal>

      {/* ============== 上传文档 Modal ============== */}
      <Modal
        title={`上传文档到「${selectedKb?.name || ''}」`}
        open={uploadModalOpen}
        onCancel={() => setUploadModalOpen(false)}
        onOk={handleUploadDoc}
        confirmLoading={uploadSubmitting}
        okText="上传并分块"
        cancelText="取消"
        destroyOnClose
        width={640}
      >
        <Alert
          message="上传后系统会自动执行:文本分块(500字符+50重叠) → Embedding 向量化 → 写入 pgvector"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
        <Form form={uploadForm} layout="vertical" preserve={false}>
          <Form.Item
            name="title"
            label="文档标题"
            rules={[{ required: true, message: '请输入文档标题' }]}
          >
            <Input placeholder="如:HD-001 负离子吹风机用户手册" maxLength={128} />
          </Form.Item>
          <Form.Item name="source" label="来源" tooltip="可选,文件路径或 URL">
            <Input placeholder="如:docs/manual-hd-001.pdf" maxLength={255} />
          </Form.Item>
          <Form.Item
            name="content"
            label="文档内容"
            rules={[{ required: true, message: '请输入文档内容' }]}
            tooltip="支持多段落(空行分隔),系统会按段落+固定长度自动分块"
          >
            <Input.TextArea
              placeholder="粘贴文档全文。系统会自动分块并向量化入库,处理过程在后台异步执行,可稍后刷新查看状态。"
              rows={10}
              showCount
              maxLength={50000}
            />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default KnowledgeBases;
