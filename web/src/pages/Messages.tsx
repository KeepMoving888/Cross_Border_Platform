/**
 * 客服消息中心页面
 * - 展示来自亚马逊 / eBay / Shopify 等平台的客户消息
 * - 接入 AI 回复建议工作流 wf_customer_service (id=3)
 *   输入: question(string), language(string, en/zh/de/fr/jp)
 *   输出: reply / intent / confidence / language / suggested_actions
 */
import React, { useMemo, useState } from 'react';
import {
  Avatar,
  Button,
  Card,
  Col,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
  Alert,
  Empty,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  RobotOutlined,
  MessageOutlined,
  ClockCircleOutlined,
  CheckCircleOutlined,
  ThunderboltOutlined,
  CopyOutlined,
  SendOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import PageContainer from '@/components/PageContainer';
import { runAIWorkflow } from '@/api/ai';
import { formatRelativeTime } from '@/utils/format';
import type { PresetColor } from '@/utils/constants';

const { Text, Paragraph } = Typography;

/** 客服消息平台(亚马逊 / eBay / Shopify) */
type CSPlatform = 'amazon' | 'ebay' | 'shopify';

/** 客服消息状态 */
type CSStatus = 'pending' | 'replied' | 'processing';

/** 客服消息本地类型(无真实 API,使用本地模拟数据) */
interface CSMessage {
  id: number;
  customer: string;
  content: string;
  platform: CSPlatform;
  status: CSStatus;
  /** ISO 时间字符串 */
  created_at: string;
}

/** AI 回复建议工作流输出结构 */
interface AIReplyResult {
  reply: string;
  intent: string;
  confidence: number;
  language: string;
  suggested_actions: string[];
}

/** 平台展示配置 */
const PLATFORM_MAP: Record<CSPlatform, { label: string; color: PresetColor }> = {
  amazon: { label: 'Amazon', color: 'orange' },
  ebay: { label: 'eBay', color: 'blue' },
  shopify: { label: 'Shopify', color: 'green' },
};

/** 状态展示配置 */
const STATUS_MAP: Record<CSStatus, { label: string; color: PresetColor }> = {
  pending: { label: '待回复', color: 'error' },
  replied: { label: '已回复', color: 'success' },
  processing: { label: '处理中', color: 'processing' },
};

/** 语言选项 */
const LANGUAGE_OPTIONS = [
  { value: 'en', label: 'English' },
  { value: 'zh', label: '中文' },
  { value: 'de', label: 'Deutsch' },
  { value: 'fr', label: 'Français' },
  { value: 'jp', label: '日本語' },
];

/** 客服消息工作流 ID(后端 wf_customer_service, id=3) */
const CS_WORKFLOW_ID = 3;

/** 生成模拟客户消息(家电美容行业场景,中英文混合) */
function buildMockMessages(): CSMessage[] {
  const now = Date.now();
  const minute = 60 * 1000;
  return [
    {
      id: 1,
      customer: 'Amazon Customer',
      content:
        'Hi, I just received the hair dryer but the plug is a US 110V one. Can I use it in Germany with 220V? Do I need an adapter or converter?',
      platform: 'amazon',
      status: 'pending',
      created_at: new Date(now - 5 * minute).toISOString(),
    },
    {
      id: 2,
      customer: 'eBay Buyer',
      content:
        'Hallo, ich habe das IPL Haarentfernungsgerät bestellt. In der Anleitung steht 5 Behandlungen pro Woche, aber mein Hautarzt meinte nur 2. Welche Empfehlung gilt?',
      platform: 'ebay',
      status: 'pending',
      created_at: new Date(now - 18 * minute).toISOString(),
    },
    {
      id: 3,
      customer: 'Shopify Customer',
      content:
        '你好,我购买的 EMS 健身仪用了三次就开不了机了,充电指示灯也不亮。订单号 #SP-88421,请问怎么申请售后或换货?',
      platform: 'shopify',
      status: 'processing',
      created_at: new Date(now - 42 * minute).toISOString(),
    },
    {
      id: 4,
      customer: 'Amazon Customer',
      content:
        'The RF beauty device arrived with a cracked screen. I have attached photos. I would like a replacement shipped to the same address.',
      platform: 'amazon',
      status: 'pending',
      created_at: new Date(now - 63 * minute).toISOString(),
    },
    {
      id: 5,
      customer: 'eBay Buyer',
      content:
        'Bonjour, le colis indique "livré" mais je n\'ai rien reçu. Le transporteur est DPD, le numéro de suivi est 0123-4567. Pouvez-vous ouvrir une enquête?',
      platform: 'ebay',
      status: 'pending',
      created_at: new Date(now - 95 * minute).toISOString(),
    },
    {
      id: 6,
      customer: 'Amazon Customer',
      content:
        '我下单的是负离子吹风机 BCD-2200W 黑色款,但收到的白色款。能否帮忙换货?包装还没拆。',
      platform: 'amazon',
      status: 'replied',
      created_at: new Date(now - 3 * 60 * minute).toISOString(),
    },
    {
      id: 7,
      customer: 'Shopify Customer',
      content:
        'Hi team, the LED face mask instruction manual is only in Chinese. Could you send me an English PDF version? Thank you!',
      platform: 'shopify',
      status: 'replied',
      created_at: new Date(now - 5 * 60 * minute).toISOString(),
    },
    {
      id: 8,
      customer: 'eBay Buyer',
      content:
        'I want to cancel order #EB-29487 for the cavitation slimming machine. It has not shipped yet. Please confirm cancellation and refund.',
      platform: 'ebay',
      status: 'processing',
      created_at: new Date(now - 7 * 60 * minute).toISOString(),
    },
    {
      id: 9,
      customer: 'Amazon Customer',
      content:
        'The depilator worked great for the first month, but now the flash intensity drops to level 1 automatically. Is this a known firmware issue?',
      platform: 'amazon',
      status: 'replied',
      created_at: new Date(now - 26 * 60 * minute).toISOString(),
    },
    {
      id: 10,
      customer: 'Shopify Customer',
      content:
        '您好,清洁美容仪的刷头配件有单独售卖吗?原装刷头用了两个月已经变形了,想再买两个备用。',
      platform: 'shopify',
      status: 'pending',
      created_at: new Date(now - 30 * 60 * minute).toISOString(),
    },
  ];
}

/** 截断文本到指定长度 */
function truncate(text: string, max = 80): string {
  if (text.length <= max) return text;
  return `${text.slice(0, max)}…`;
}

/** 取客户名首字母作为 Avatar 文本 */
function avatarText(name: string): string {
  return name.charAt(0).toUpperCase();
}

const Messages: React.FC = () => {
  const [messages, setMessages] = useState<CSMessage[]>(() => buildMockMessages());
  const [modalOpen, setModalOpen] = useState(false);
  const [activeMessage, setActiveMessage] = useState<CSMessage | null>(null);
  const [language, setLanguage] = useState<string>('en');
  const [loading, setLoading] = useState(false);
  const [aiResult, setAiResult] = useState<AIReplyResult | null>(null);

  /** 顶部统计(基于当前消息列表 + 模拟指标) */
  const stats = useMemo(() => {
    const pending = messages.filter((m) => m.status === 'pending').length;
    const repliedToday = messages.filter((m) => m.status === 'replied').length;
    return {
      pending,
      repliedToday,
      avgResponseMin: 12,
      adoptionRate: 0.78,
    };
  }, [messages]);

  /** 刷新模拟数据 */
  const handleRefresh = () => {
    setMessages(buildMockMessages());
    message.success('消息列表已刷新');
  };

  /** 打开 AI 回复 Modal */
  const handleOpenAI = (record: CSMessage) => {
    setActiveMessage(record);
    // 根据消息内容粗略推断默认语言(含中文/日文/德文/法文字符则用对应语言,否则 en)
    if (/[\u4e00-\u9fa5]/.test(record.content)) {
      setLanguage('zh');
    } else if (/[\u3040-\u30ff]/.test(record.content)) {
      setLanguage('jp');
    } else if (/[äöüÄÖÜß]/.test(record.content)) {
      setLanguage('de');
    } else if (/[àâçéèêëîïôûùüÿœÀÂÇÉÈÊËÎÏÔÛÙÜŸŒ]/.test(record.content)) {
      setLanguage('fr');
    } else {
      setLanguage('en');
    }
    setAiResult(null);
    setModalOpen(true);
  };

  /** 调用 AI 工作流生成回复建议 */
  const handleGenerate = async () => {
    if (!activeMessage) return;
    setLoading(true);
    try {
      const result = await runAIWorkflow(CS_WORKFLOW_ID, {
        input: {
          question: activeMessage.content,
          language,
        },
      });
      const parsed = JSON.parse(result.output) as AIReplyResult;
      // 兼容部分工作流返回 { parsed: {...} } 的双层结构
      const inner = (parsed as unknown as { parsed?: AIReplyResult }).parsed || parsed;
      setAiResult({
        reply: inner.reply || '',
        intent: inner.intent || 'general',
        confidence: typeof inner.confidence === 'number' ? inner.confidence : 0,
        language: inner.language || language,
        suggested_actions: Array.isArray(inner.suggested_actions) ? inner.suggested_actions : [],
      });
      message.success('AI 回复建议已生成');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'AI 回复生成失败,请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  /** 复制回复到剪贴板 */
  const handleCopy = async () => {
    if (!aiResult?.reply) return;
    try {
      await navigator.clipboard.writeText(aiResult.reply);
      message.success('回复内容已复制到剪贴板');
    } catch {
      message.error('复制失败,请手动选中复制');
    }
  };

  /** 发送并标记为已回复 */
  const handleSendAndMark = () => {
    if (!activeMessage) return;
    setMessages((prev) =>
      prev.map((m) => (m.id === activeMessage.id ? { ...m, status: 'replied' } : m)),
    );
    message.success('回复已发送,消息已标记为「已回复」');
    setModalOpen(false);
    setActiveMessage(null);
    setAiResult(null);
  };

  /** 表格列定义 */
  const columns: ColumnsType<CSMessage> = [
    {
      title: '客户',
      dataIndex: 'customer',
      width: 200,
      render: (_, record) => (
        <Space>
          <Avatar style={{ backgroundColor: '#722ed1' }}>{avatarText(record.customer)}</Avatar>
          <Text strong>{record.customer}</Text>
        </Space>
      ),
    },
    {
      title: '消息内容',
      dataIndex: 'content',
      ellipsis: true,
      render: (content: string) => (
        <Text style={{ color: 'rgba(0,0,0,0.75)' }}>{truncate(content)}</Text>
      ),
    },
    {
      title: '平台',
      dataIndex: 'platform',
      width: 110,
      render: (platform: CSPlatform) => (
        <Tag color={PLATFORM_MAP[platform].color} style={{ borderRadius: 12 }}>
          {PLATFORM_MAP[platform].label}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (status: CSStatus) => (
        <Tag color={STATUS_MAP[status].color} style={{ borderRadius: 12 }}>
          {STATUS_MAP[status].label}
        </Tag>
      ),
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      width: 120,
      render: (created_at: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {formatRelativeTime(created_at)}
        </Text>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 130,
      fixed: 'right',
      render: (_, record) => (
        <Button
          type="primary"
          icon={<RobotOutlined />}
          size="small"
          onClick={() => handleOpenAI(record)}
        >
          AI 回复
        </Button>
      ),
    },
  ];

  return (
    <PageContainer title="客服消息中心" breadcrumb={{}}>
      {/* 顶部统计卡片 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={12} md={6}>
          <Card bordered={false} className="cbp-metric-tile">
            <Statistic
              title="待回复消息"
              value={stats.pending}
              prefix={<MessageOutlined style={{ color: '#cf1322' }} />}
              valueStyle={{ color: '#cf1322' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card bordered={false} className="cbp-metric-tile">
            <Statistic
              title="今日已回复"
              value={stats.repliedToday}
              prefix={<CheckCircleOutlined style={{ color: '#52c41a' }} />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card bordered={false} className="cbp-metric-tile">
            <Statistic
              title="平均响应时间(分钟)"
              value={stats.avgResponseMin}
              prefix={<ClockCircleOutlined style={{ color: '#1677ff' }} />}
              valueStyle={{ color: '#1677ff' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={12} md={6}>
          <Card bordered={false} className="cbp-metric-tile">
            <Statistic
              title="AI 回复采用率"
              value={stats.adoptionRate * 100}
              precision={1}
              suffix="%"
              prefix={<ThunderboltOutlined style={{ color: '#722ed1' }} />}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
      </Row>

      {/* 消息列表 */}
      <Card
        bordered={false}
        title="客户消息列表"
        extra={
          <Button icon={<ReloadOutlined />} onClick={handleRefresh}>
            刷新
          </Button>
        }
      >
        <Table<CSMessage>
          rowKey="id"
          columns={columns}
          dataSource={messages}
          pagination={{ pageSize: 10, showSizeChanger: true }}
          scroll={{ x: 1100 }}
          locale={{
            emptyText: <Empty description="暂无客户消息" />,
          }}
        />
      </Card>

      {/* AI 回复 Modal */}
      <Modal
        title={
          <Space>
            <RobotOutlined style={{ color: '#722ed1' }} />
            <span>AI 回复建议</span>
          </Space>
        }
        open={modalOpen}
        onCancel={() => {
          setModalOpen(false);
          setActiveMessage(null);
          setAiResult(null);
        }}
        footer={null}
        width={680}
        destroyOnClose
      >
        {activeMessage && (
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            {/* 原始客户消息(引用样式) */}
            <Card
              size="small"
              bordered={false}
              style={{ background: '#fafafa', borderLeft: '3px solid #1677ff' }}
            >
              <Space size={8} style={{ marginBottom: 6 }}>
                <Avatar size="small" style={{ backgroundColor: '#722ed1' }}>
                  {avatarText(activeMessage.customer)}
                </Avatar>
                <Text strong>{activeMessage.customer}</Text>
                <Tag color={PLATFORM_MAP[activeMessage.platform].color}>
                  {PLATFORM_MAP[activeMessage.platform].label}
                </Tag>
              </Space>
              <Paragraph style={{ margin: 0, color: 'rgba(0,0,0,0.75)', whiteSpace: 'pre-wrap' }}>
                {activeMessage.content}
              </Paragraph>
            </Card>

            {/* 语言选择 + 生成按钮 */}
            <Space wrap style={{ width: '100%' }}>
              <Text>回复语言:</Text>
              <Select
                value={language}
                onChange={setLanguage}
                options={LANGUAGE_OPTIONS}
                style={{ width: 160 }}
              />
              <Button
                type="primary"
                icon={<RobotOutlined />}
                loading={loading}
                onClick={handleGenerate}
                style={{ background: '#722ed1', borderColor: '#722ed1' }}
              >
                {aiResult ? '重新生成' : '生成 AI 回复'}
              </Button>
            </Space>

            {/* 生成结果区 */}
            {aiResult && (
              <Card size="small" bordered={false} style={{ background: '#f6ffed', border: '1px solid #b7eb8f' }}>
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Alert
                    type="info"
                    showIcon
                    message="建议回复"
                    description={
                      <Paragraph style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
                        {aiResult.reply}
                      </Paragraph>
                    }
                  />

                  {/* 标签行:意图 / 置信度 / 语言 */}
                  <Space wrap>
                    <Tag color="purple">意图:{aiResult.intent}</Tag>
                    <Tag color="blue">置信度:{(aiResult.confidence * 100).toFixed(1)}%</Tag>
                    <Tag color="cyan">语言:{aiResult.language}</Tag>
                  </Space>

                  {/* 建议动作 */}
                  {aiResult.suggested_actions.length > 0 && (
                    <div>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        建议动作:
                      </Text>
                      <div style={{ marginTop: 6 }}>
                        <Space wrap>
                          {aiResult.suggested_actions.map((action, idx) => (
                            <Tag key={idx} color="gold" style={{ borderRadius: 12 }}>
                              {action}
                            </Tag>
                          ))}
                        </Space>
                      </div>
                    </div>
                  )}

                  {/* 操作按钮 */}
                  <Space>
                    <Button icon={<CopyOutlined />} onClick={handleCopy}>
                      复制回复
                    </Button>
                    <Button
                      type="primary"
                      icon={<SendOutlined />}
                      onClick={handleSendAndMark}
                      style={{ background: '#52c41a', borderColor: '#52c41a' }}
                    >
                      发送并标记已回复
                    </Button>
                  </Space>
                </Space>
              </Card>
            )}
          </Space>
        )}
      </Modal>
    </PageContainer>
  );
};

export default Messages;
