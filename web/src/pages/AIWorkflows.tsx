/**
 * AI 工作流:企业级卡片网格 + 参数执行 + 结构化结果展示
 * 当前执行链路使用本地模拟数据(未接入 LLM Key)
 */
import React, { useEffect, useMemo, useState } from 'react';
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
  Spin,
  Empty,
  Divider,
  Descriptions,
  message,
  Progress,
  Alert,
  List,
  Statistic,
} from 'antd';
import {
  RobotOutlined,
  PlayCircleOutlined,
  CheckCircleFilled,
  ThunderboltOutlined,
  ExperimentOutlined,
  LineChartOutlined,
  ScheduleOutlined,
  SafetyCertificateOutlined,
  BulbOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { Link } from 'react-router-dom';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { listAIWorkflows, runAIWorkflow } from '@/api/ai';
import { AI_WORKFLOW_TYPE_MAP, AI_WORKFLOW_STATUS_MAP } from '@/utils/constants';
import { formatDateTime, formatNumber } from '@/utils/format';
import type { AIWorkflow, AIWorkflowRunResult } from '@/types/api';

const { Text, Paragraph, Title } = Typography;

const TYPE_ICON_MAP: Record<string, React.ReactNode> = {
  agent: <ExperimentOutlined />,
  automation: <ThunderboltOutlined />,
  rag: <LineChartOutlined />,
  text2sql: <ScheduleOutlined />,
};

const SCENE_LABEL: Record<string, string> = {
  product_analysis: '选品分析',
  purchase_assistant: '采购助手',
  customer_service: '智能客服',
  data_analysis: '数据分析',
  content_generation: '内容生成',
};

/** 各工作流的输入参数 schema(对齐后端 input_schema,按 wf.code 索引) */
const WORKFLOW_INPUT_SCHEMA: Record<
  string,
  Array<{ field: string; label: string; placeholder: string; required?: boolean; defaultValue?: string }>
> = {
  wf_product_analysis: [
    { field: 'sku', label: 'SKU', placeholder: '请输入 SKU', defaultValue: 'PC-HD-001', required: true },
    { field: 'name', label: '商品名称', placeholder: '请输入商品名称', defaultValue: '负离子恒温高速吹风机', required: true },
    { field: 'category', label: '目标类目', placeholder: '如:beauty_device', defaultValue: 'beauty_device', required: true },
    { field: 'list_price', label: '售价(USD)', placeholder: '如:39.99', defaultValue: '39.99' },
    { field: 'est_cost_price', label: '预估成本(USD)', placeholder: '如:14.50', defaultValue: '14.50' },
    { field: 'monthly_sales', label: '月销量', placeholder: '如:1200', defaultValue: '1200' },
  ],
  wf_purchase_assistant: [
    { field: 'order_no', label: '采购单号', placeholder: '如:PO-0001', defaultValue: 'PO-0001', required: true },
    { field: 'unit_price', label: '供应商单价(USD)', placeholder: '如:18.50', defaultValue: '18.50', required: true },
    { field: 'quantity', label: '采购数量', placeholder: '如:500', defaultValue: '500', required: true },
  ],
  wf_customer_service: [
    {
      field: 'question',
      label: '客户问题',
      placeholder: '如:Does this hair dryer support dual voltage?',
      defaultValue: 'Does this hair dryer support dual voltage?',
      required: true,
    },
    { field: 'language', label: '回复语言', placeholder: '如:en / de / fr / jp', defaultValue: 'en' },
  ],
  wf_data_analysis: [
    {
      field: 'question',
      label: '分析问题',
      placeholder: '如:近30天哪些SKU利润最高?',
      defaultValue: '近30天哪些SKU利润最高?',
      required: true,
    },
  ],
  wf_content_generation: [
    { field: 'sku', label: 'SKU', placeholder: '请输入 SKU', defaultValue: 'PC-HD-001', required: true },
    { field: 'name', label: '商品名称', placeholder: '请输入商品名称', defaultValue: '负离子恒温高速吹风机', required: true },
    { field: 'category', label: '类目', placeholder: '如:beauty_device', defaultValue: 'beauty_device', required: true },
  ],
};

function parseOutput(output?: string): Record<string, any> | null {
  if (!output) return null;
  try {
    return JSON.parse(output);
  } catch {
    return null;
  }
}

const RecommendationTag: React.FC<{ value?: string }> = ({ value }) => {
  if (!value) return null;
  const color = value.includes('推荐') ? 'success' : value.includes('谨慎') ? 'warning' : 'error';
  return (
    <Tag color={color} style={{ borderRadius: 6, fontSize: 14, padding: '4px 12px', lineHeight: '22px' }}>
      {value}
    </Tag>
  );
};

/** 将查询结果数组导出为 CSV 文件 */
const exportResultToCSV = (rows: any[], filename = 'query_result.csv') => {
  if (!rows.length) return;
  const keys = Object.keys(rows[0]);
  const escape = (v: any) => {
    if (v === null || v === undefined) return '';
    const s = String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const csv = [keys.join(','), ...rows.map((r) => keys.map((k) => escape(r[k])).join(','))].join('\n');
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
};

const StructuredResult: React.FC<{ result: AIWorkflowRunResult; code?: string }> = ({ result, code }) => {
  const data = useMemo(() => parseOutput(result.output), [result.output]);

  if (!data) {
    return <div className="cbp-ai-insight">{result.output}</div>;
  }

  if (code === 'wf_product_analysis' || data.score !== undefined) {
    const score = Number(data.score || 0);
    return (
      <div>
        <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
          <Col xs={24} md={10}>
            <Card className="cbp-result-hero" bordered={false}>
              <div style={{ textAlign: 'center' }}>
                <div style={{ color: 'rgba(0,0,0,0.45)', marginBottom: 8 }}>综合评分</div>
                <Progress
                  type="dashboard"
                  percent={score}
                  strokeColor={score >= 80 ? '#52c41a' : score >= 65 ? '#faad14' : '#ff4d4f'}
                  format={(p) => <span style={{ fontSize: 28, fontWeight: 700 }}>{p}</span>}
                />
                <div style={{ marginTop: 8 }}>
                  <RecommendationTag value={data.recommendation} />
                </div>
              </div>
            </Card>
          </Col>
          <Col xs={24} md={14}>
            <Alert
              type={score >= 80 ? 'success' : score >= 65 ? 'warning' : 'error'}
              showIcon
              icon={<BulbOutlined />}
              message="关键结论"
              description={<Text strong style={{ fontSize: 15 }}>{data.suggestion || data.recommendation}</Text>}
              style={{ borderRadius: 10, marginBottom: 12 }}
            />
            <Row gutter={12}>
              <Col span={8}>
                <Statistic title="预估毛利率" value={data.metrics?.est_margin_rate ?? '-'} suffix="%" />
              </Col>
              <Col span={8}>
                <Statistic title="售价" prefix="$" value={data.metrics?.list_price ?? '-'} precision={2} />
              </Col>
              <Col span={8}>
                <Statistic title="月销参考" value={data.metrics?.monthly_sales ?? '-'} />
              </Col>
            </Row>
          </Col>
        </Row>
        <Row gutter={16}>
          <Col xs={24} md={12}>
            <Card size="small" title={<Space><CheckCircleFilled style={{ color: '#52c41a' }} />推荐理由</Space>} style={{ borderRadius: 10 }}>
              <List
                size="small"
                dataSource={data.reasons || []}
                renderItem={(item: string) => <List.Item style={{ padding: '8px 0' }}>{item}</List.Item>}
              />
            </Card>
          </Col>
          <Col xs={24} md={12}>
            <Card size="small" title={<Space><WarningOutlined style={{ color: '#faad14' }} />风险提示</Space>} style={{ borderRadius: 10 }}>
              <List
                size="small"
                dataSource={data.risks || []}
                renderItem={(item: string) => <List.Item style={{ padding: '8px 0' }}>{item}</List.Item>}
              />
            </Card>
          </Col>
        </Row>
      </div>
    );
  }

  if (code === 'wf_purchase_assistant' || data.negotiation) {
    return (
      <div>
        <Alert
          type={data.price_reasonable ? 'success' : 'warning'}
          showIcon
          message={data.price_reasonable ? '报价合理' : '报价偏高，建议继续谈判'}
          description={<Text strong>{data.negotiation}</Text>}
          style={{ borderRadius: 10, marginBottom: 16 }}
        />
        <Row gutter={16} style={{ marginBottom: 16 }}>
          <Col span={8}><Statistic title="报价" prefix="$" value={data.quote_price} precision={2} /></Col>
          <Col span={8}><Statistic title="市场均价" prefix="$" value={data.market_avg} precision={2} /></Col>
          <Col span={8}><Statistic title="交付风险" value={data.delivery_risk === 'low' ? '低' : data.delivery_risk === 'medium' ? '中' : '高'} /></Col>
        </Row>
        <Card size="small" title="执行检查清单" style={{ borderRadius: 10 }}>
          <List size="small" dataSource={data.checklist || []} renderItem={(item: string) => <List.Item>{item}</List.Item>} />
        </Card>
      </div>
    );
  }

  if (code === 'wf_customer_service' || data.reply) {
    return (
      <div>
        <Alert type="info" showIcon message="建议回复" description={<Paragraph style={{ marginBottom: 0 }}>{data.reply}</Paragraph>} style={{ borderRadius: 10, marginBottom: 16 }} />
        <Space wrap>
          <Tag color="blue">意图: {data.intent || '-'}</Tag>
          <Tag color="green">置信度: {Math.round(Number(data.confidence || 0) * 100)}%</Tag>
          <Tag>{data.language || 'en'}</Tag>
        </Space>
        {Array.isArray(data.suggested_actions) && (
          <div style={{ marginTop: 16 }}>
            <Text type="secondary">建议动作</Text>
            <div style={{ marginTop: 8 }}>
              <Space wrap>
                {data.suggested_actions.map((a: string) => (
                  <Tag key={a} color="purple">{a}</Tag>
                ))}
              </Space>
            </div>
          </div>
        )}
      </div>
    );
  }

  if (code === 'wf_data_analysis' || data.insight) {
    return (
      <div>
        <Alert type="success" showIcon message="业务洞察" description={<Text strong>{data.insight}</Text>} style={{ borderRadius: 10, marginBottom: 16 }} />
        {data.sql && (
          <Card size="small" title="生成 SQL" style={{ borderRadius: 10, marginBottom: 16 }}>
            <pre className="cbp-code-block">{data.sql}</pre>
          </Card>
        )}
        {Array.isArray(data.result) && data.result.length > 0 && (
          <Card
            size="small"
            style={{ borderRadius: 10 }}
            title={
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span>查询结果预览</span>
                <Button size="small" type="link" onClick={() => exportResultToCSV(data.result)}>
                  导出 CSV
                </Button>
              </div>
            }
          >
            <Table
              size="small"
              rowKey={(_, idx) => String(idx)}
              dataSource={data.result}
              pagination={{ pageSize: 10, showSizeChanger: false }}
              scroll={{ x: 'max-content' }}
              columns={Object.keys(data.result[0]).map((key) => {
                // 数值列判定:该列所有非空值均可被 Number 解析
                const isNumeric = (data.result as any[]).every(
                  (r) => r[key] === null || r[key] === undefined || r[key] === '' || !isNaN(Number(r[key])),
                );
                return {
                  title: key,
                  dataIndex: key,
                  key,
                  align: isNumeric ? 'right' : 'left',
                  sorter: (a: any, b: any) =>
                    isNumeric
                      ? Number(a[key] || 0) - Number(b[key] || 0)
                      : String(a[key] ?? '').localeCompare(String(b[key] ?? '')),
                  render: (val: any) =>
                    isNumeric && val !== null && val !== undefined && val !== ''
                      ? Number(val).toLocaleString()
                      : val,
                };
              })}
            />
          </Card>
        )}
      </div>
    );
  }

  if (code === 'wf_content_generation' || data.title) {
    return (
      <div>
        <Card size="small" title="Listing 标题" style={{ borderRadius: 10, marginBottom: 12 }}>
          <Text strong style={{ fontSize: 15 }}>{data.title}</Text>
        </Card>
        <Card size="small" title="五点描述" style={{ borderRadius: 10, marginBottom: 12 }}>
          <List
            size="small"
            dataSource={data.bullets || []}
            renderItem={(item: string, idx: number) => (
              <List.Item>
                <Text type="secondary" style={{ marginRight: 8 }}>{idx + 1}.</Text>
                {item}
              </List.Item>
            )}
          />
        </Card>
        <Card size="small" title="产品描述" style={{ borderRadius: 10, marginBottom: 12 }}>
          <Paragraph style={{ marginBottom: 0 }}>{data.description}</Paragraph>
        </Card>
        <Space wrap>
          {(data.keywords || []).map((k: string) => (
            <Tag key={k} color="blue">{k}</Tag>
          ))}
        </Space>
      </div>
    );
  }

  return <pre className="cbp-code-block">{JSON.stringify(data, null, 2)}</pre>;
};

const AIWorkflows: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [workflows, setWorkflows] = useState<AIWorkflow[]>([]);
  const [runModal, setRunModal] = useState<{ open: boolean; wf?: AIWorkflow }>({ open: false });
  const [resultModal, setResultModal] = useState<{ open: boolean; result?: AIWorkflowRunResult; wf?: AIWorkflow }>({
    open: false,
  });
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const res = await listAIWorkflows();
        setWorkflows(res?.list || []);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const openRunModal = (wf: AIWorkflow) => {
    if (wf.status === 'disabled') {
      message.warning('该工作流当前已停用,请联系管理员启用');
      return;
    }
    setRunModal({ open: true, wf });
    const schema = WORKFLOW_INPUT_SCHEMA[wf.code] || [];
    const initial: Record<string, string> = {};
    schema.forEach((s) => {
      if (s.defaultValue) initial[s.field] = s.defaultValue;
    });
    form.setFieldsValue(initial);
  };

  const handleRun = async () => {
    if (!runModal.wf) return;
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const result = await runAIWorkflow(runModal.wf.id, { input: values }, { code: runModal.wf.code });
      setRunModal({ open: false });
      form.resetFields();
      setResultModal({ open: true, result, wf: runModal.wf });
      message.success('工作流执行完成');
    } catch (err) {
      message.error(err instanceof Error ? err.message : '工作流执行失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <PageContainer
      title="AI 工作流"
      breadcrumb={{}}
      subTitle=""
    >
      <Alert
        type="success"
        showIcon
        icon={<SafetyCertificateOutlined />}
        message="已接入 AI 工作流引擎"
        description={<>点击「立即执行」将真实调用后端工作流引擎,按节点拓扑执行(input → LLM/RAG/Text2SQL → output)。未配置 LLM API Key 时使用内置 Provider 返回结构化结果,配置后无缝切换至 GLM/Claude/DeepSeek/Qwen 真实推理。<Link to="/ai/runs">查看历史记录 →</Link></>}
        style={{ marginBottom: 16, borderRadius: 10 }}
      />

      <Spin spinning={loading}>
        {workflows.length === 0 && !loading ? (
          <Empty description="暂无可用工作流" />
        ) : (
          <Row gutter={[16, 16]}>
            {workflows.map((wf) => (
              <Col xs={24} sm={12} xl={8} key={wf.id}>
                <Card
                  className="cbp-workflow-card cbp-enterprise-card"
                  bordered={false}
                  styles={{ body: { padding: 20, display: 'flex', flexDirection: 'column', height: '100%' } }}
                >
                  <Space align="start" style={{ width: '100%' }}>
                    <div className="cbp-icon-tile">
                      {TYPE_ICON_MAP[wf.type] || <RobotOutlined />}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <Title level={5} style={{ margin: 0, fontWeight: 600 }}>
                        {wf.name}
                      </Title>
                      <Space size={6} style={{ marginTop: 6 }} wrap>
                        <Text type="secondary" style={{ fontSize: 12 }}>{wf.code}</Text>
                        <StatusTag status={wf.type} map={AI_WORKFLOW_TYPE_MAP} />
                      </Space>
                    </div>
                  </Space>

                  <Paragraph
                    type="secondary"
                    style={{ marginTop: 14, marginBottom: 12, minHeight: 60, fontSize: 13, lineHeight: 1.7 }}
                    ellipsis={{ rows: 3 }}
                  >
                    {wf.description}
                  </Paragraph>

                  <div style={{ marginTop: 'auto' }}>
                    <Divider style={{ margin: '8px 0 12px' }} />
                    <Space style={{ width: '100%', justifyContent: 'space-between' }} align="center">
                      <Space size={6} wrap>
                        <Tag bordered={false} color="blue">{SCENE_LABEL[wf.scene] || wf.scene}</Tag>
                        <StatusTag status={wf.status} map={AI_WORKFLOW_STATUS_MAP} />
                      </Space>
                      <Button
                        type="primary"
                        size="middle"
                        icon={<PlayCircleOutlined />}
                        onClick={() => openRunModal(wf)}
                        disabled={wf.status === 'disabled'}
                      >
                        立即执行
                      </Button>
                    </Space>
                  </div>
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Spin>

      <Modal
        title={
          <Space>
            <RobotOutlined style={{ color: '#1677ff' }} />
            <span>执行工作流 · {runModal.wf?.name}</span>
          </Space>
        }
        open={runModal.open}
        onCancel={() => {
          setRunModal({ open: false });
          form.resetFields();
        }}
        onOk={handleRun}
        confirmLoading={submitting}
        okText={submitting ? '执行中' : '提交执行'}
        cancelText="取消"
        width={560}
        destroyOnClose
      >
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          {runModal.wf?.description}
        </Paragraph>
        {submitting && (
          <Spin tip="正在调用 AI 工作流引擎,请稍候..." style={{ display: 'block', marginBottom: 16 }}>
            <div style={{ height: 24 }} />
          </Spin>
        )}
        <Form form={form} layout="vertical" preserve={false}>
          {(WORKFLOW_INPUT_SCHEMA[runModal.wf?.code ?? ''] || []).map((field) => (
            <Form.Item
              key={field.field}
              name={field.field}
              label={field.label}
              rules={field.required === false ? [] : [{ required: true, message: `请输入${field.label}` }]}
            >
              <Input placeholder={field.placeholder} />
            </Form.Item>
          ))}
        </Form>
      </Modal>

      <Modal
        title={
          <Space>
            <CheckCircleFilled style={{ color: '#52c41a' }} />
            <span>执行结果 · {resultModal.wf?.name}</span>
          </Space>
        }
        open={resultModal.open}
        onCancel={() => setResultModal({ open: false })}
        footer={[
          <Button
            key="copy"
            onClick={() => {
              if (resultModal.result?.output) {
                navigator.clipboard
                  .writeText(resultModal.result.output)
                  .then(() => message.success('已复制到剪贴板'))
                  .catch(() => message.error('复制失败'));
              }
            }}
          >
            复制 JSON
          </Button>,
          <Button
            key="rerun"
            onClick={() => {
              const wf = resultModal.wf;
              setResultModal({ open: false });
              if (wf) openRunModal(wf);
            }}
          >
            重新执行
          </Button>,
          <Button key="close" type="primary" onClick={() => setResultModal({ open: false })}>
            关闭
          </Button>,
        ]}
        width={820}
      >
        {resultModal.result && (
          <>
            <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="执行状态">
                <Tag color={resultModal.result.status === 'success' ? 'success' : 'error'}>
                  {resultModal.result.status === 'success' ? '执行成功' : '执行失败'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="运行 ID">{resultModal.result.run_id}</Descriptions.Item>
              <Descriptions.Item label="耗时">{formatNumber(resultModal.result.duration_ms)} ms</Descriptions.Item>
              <Descriptions.Item label="完成时间">{formatDateTime(resultModal.result.finished_at)}</Descriptions.Item>
            </Descriptions>
            {resultModal.result.metrics && (
              <Space size={8} style={{ marginBottom: 12 }} wrap>
                {Object.entries(resultModal.result.metrics).map(([k, v]) => (
                  <Tag key={k} color="blue">{k}: {String(v)}</Tag>
                ))}
              </Space>
            )}
            <Title level={5} style={{ marginTop: 8, marginBottom: 12 }}>结构化输出</Title>
            <StructuredResult result={resultModal.result} code={resultModal.wf?.code} />
          </>
        )}
      </Modal>
    </PageContainer>
  );
};

export default AIWorkflows;
