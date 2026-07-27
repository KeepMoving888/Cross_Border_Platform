/**
 * AI 工作流历史记录:顶部统计卡 + 筛选区 + 趋势图 + 历史表格 + 详情 Modal
 * 对接后端 GET /api/v1/ai/runs(workflow_code/status/page/page_size)
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react';
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
  Select,
  Spin,
  Empty,
  Descriptions,
  message,
  Statistic,
} from 'antd';
import {
  CheckCircleOutlined,
  ThunderboltOutlined,
  ClockCircleOutlined,
  FireOutlined,
  ReloadOutlined,
  EyeOutlined,
  CopyOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { listAIRuns } from '@/api/ai';
import { formatDateTime, formatNumber } from '@/utils/format';
import type { AIWorkflowRun, PageResponse } from '@/types/api';
import type { PresetColor } from '@/utils/constants';

const { Text, Title } = Typography;

/** 工作流代码 -> 中文名称映射 */
const SCENE_LABEL: Record<string, string> = {
  wf_product_analysis: '选品分析',
  wf_purchase_assistant: '采购助手',
  wf_customer_service: '智能客服',
  wf_data_analysis: '数据分析',
  wf_content_generation: '内容生成',
};

/** 运行状态字典(success=绿/failed=红/running=蓝/timeout=橙) */
const RUN_STATUS_MAP: Record<string, { label: string; color: PresetColor }> = {
  success: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
  running: { label: '运行中', color: 'processing' },
  timeout: { label: '超时', color: 'warning' },
};

/** 触发类型字典 */
const TRIGGER_TYPE_MAP: Record<string, { label: string; color: PresetColor }> = {
  manual: { label: '手动', color: 'blue' },
  scheduled: { label: '定时', color: 'purple' },
  event: { label: '事件', color: 'cyan' },
};

/** 耗时格式化:<1s 显示 "123 ms",>=1s 显示 "1.2 s" */
function formatDuration(ms: number | undefined | null): string {
  if (ms === undefined || ms === null) return '-';
  const n = Number(ms);
  if (Number.isNaN(n)) return '-';
  if (n < 1000) return `${Math.round(n)} ms`;
  return `${(n / 1000).toFixed(1)} s`;
}

/** 安全解析 JSON 字符串,失败则原样返回字符串 */
function safeParseJSON(text?: string): unknown {
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

const AIWorkflowRuns: React.FC = () => {
  // 列表与分页状态
  const [loading, setLoading] = useState(false);
  const [runs, setRuns] = useState<AIWorkflowRun[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // 全量记录(用于统计卡与趋势图聚合,拉取较大一页)
  const [allRuns, setAllRuns] = useState<AIWorkflowRun[]>([]);

  // 筛选表单状态
  const [form] = Form.useForm();
  const [filters, setFilters] = useState<{
    workflow_code?: string;
    status?: string;
    trigger_type?: string;
  }>({});

  // 详情 Modal
  const [detailOpen, setDetailOpen] = useState(false);
  const [current, setCurrent] = useState<AIWorkflowRun | null>(null);

  /** 拉取分页列表(后端仅支持 workflow_code/status 过滤) */
  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = (await listAIRuns({
        workflow_code: filters.workflow_code,
        status: filters.status,
        page,
        page_size: pageSize,
      })) as unknown as PageResponse<AIWorkflowRun>;
      setRuns(res?.list || []);
      setTotal(res?.total || 0);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载历史记录失败');
    } finally {
      setLoading(false);
    }
  }, [filters.workflow_code, filters.status, page, pageSize]);

  /** 拉取全量记录用于统计卡与趋势图聚合 */
  const fetchAllForTrend = useCallback(async () => {
    try {
      const res = (await listAIRuns({
        page: 1,
        page_size: 500,
      })) as unknown as PageResponse<AIWorkflowRun>;
      setAllRuns(res?.list || []);
    } catch {
      // 趋势图为可选增强,失败不阻塞主流程
    }
  }, []);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  useEffect(() => {
    fetchAllForTrend();
  }, [fetchAllForTrend]);

  /** 查询按钮 */
  const onSearch = () => {
    const values = form.getFieldsValue();
    setFilters({
      workflow_code: (values.workflow_code as string)?.trim() || undefined,
      status: values.status && values.status !== 'all' ? values.status : undefined,
      trigger_type: values.trigger_type && values.trigger_type !== 'all' ? values.trigger_type : undefined,
    });
    setPage(1);
  };

  /** 重置按钮 */
  const onReset = () => {
    form.resetFields();
    setFilters({});
    setPage(1);
  };

  // 顶部统计指标(基于全量记录聚合)
  const stats = useMemo(() => {
    const list = allRuns;
    const totalCount = list.length;
    const successCount = list.filter((r) => r.status === 'success').length;
    const successRate = totalCount > 0 ? (successCount / totalCount) * 100 : 0;
    const avgDuration =
      totalCount > 0 ? list.reduce((sum, r) => sum + Number(r.duration_ms || 0), 0) / totalCount : 0;
    const totalTokens = list.reduce((sum, r) => sum + Number(r.total_tokens || 0), 0);
    return { totalCount, successCount, successRate, avgDuration, totalTokens };
  }, [allRuns]);

  // 触发类型在后端 API 不支持,这里对当前页做客户端过滤
  const displayRuns = useMemo(() => {
    if (!filters.trigger_type) return runs;
    return runs.filter((r) => r.trigger_type === filters.trigger_type);
  }, [runs, filters.trigger_type]);

  /** 趋势图配置:最近 30 天每日执行次数与成功率 */
  const trendOption: EChartsOption | null = useMemo(() => {
    if (allRuns.length === 0) return null;
    // 构造最近 30 天的日期桶
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const dateBuckets: { date: string; total: number; success: number }[] = [];
    for (let i = 29; i >= 0; i--) {
      const d = new Date(today);
      d.setDate(d.getDate() - i);
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(
        d.getDate(),
      ).padStart(2, '0')}`;
      dateBuckets.push({ date: key, total: 0, success: 0 });
    }
    const bucketMap = new Map(dateBuckets.map((b) => [b.date, b]));
    allRuns.forEach((r) => {
      const raw = r.created_at || r.started_at || '';
      if (!raw) return;
      const key = raw.slice(0, 10);
      const b = bucketMap.get(key);
      if (b) {
        b.total += 1;
        if (r.status === 'success') b.success += 1;
      }
    });

    const xAxisData = dateBuckets.map((b) => b.date.slice(5)); // MM-DD
    const countData = dateBuckets.map((b) => b.total);
    const rateData = dateBuckets.map((b) =>
      b.total > 0 ? Number(((b.success / b.total) * 100).toFixed(1)) : 0,
    );

    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(255,255,255,0.98)',
        borderColor: '#eef0f4',
        borderWidth: 1,
        extraCssText: 'box-shadow: 0 6px 16px rgba(0,0,0,0.08); border-radius: 8px;',
      },
      legend: {
        data: ['执行次数', '成功率'],
        top: 4,
        right: 8,
        icon: 'roundRect',
        itemWidth: 12,
        itemHeight: 8,
        textStyle: { fontSize: 12, color: 'rgba(0,0,0,0.65)' },
      },
      grid: { left: 48, right: 48, top: 44, bottom: 32 },
      xAxis: {
        type: 'category',
        data: xAxisData,
        boundaryGap: false,
        axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
      },
      yAxis: [
        {
          type: 'value',
          name: '次数',
          nameTextStyle: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        },
        {
          type: 'value',
          name: '成功率',
          min: 0,
          max: 100,
          nameTextStyle: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)', formatter: '{value}%' },
          splitLine: { show: false },
        },
      ],
      series: [
        {
          name: '执行次数',
          type: 'line',
          smooth: true,
          showSymbol: false,
          yAxisIndex: 0,
          data: countData,
          itemStyle: { color: '#1677ff' },
          lineStyle: { width: 2.5 },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(22,119,255,0.22)' },
                { offset: 1, color: 'rgba(22,119,255,0.02)' },
              ],
            },
          },
        },
        {
          name: '成功率',
          type: 'line',
          smooth: true,
          showSymbol: false,
          yAxisIndex: 1,
          data: rateData,
          itemStyle: { color: '#52c41a' },
          lineStyle: { width: 2.5 },
        },
      ],
    };
  }, [allRuns]);

  /** 表格列定义 */
  const columns = [
    {
      title: '运行 ID',
      dataIndex: 'id',
      key: 'id',
      width: 90,
      fixed: 'left' as const,
    },
    {
      title: '工作流',
      dataIndex: 'workflow_code',
      key: 'workflow_code',
      width: 160,
      render: (code: string) => SCENE_LABEL[code] || code,
    },
    {
      title: '触发类型',
      dataIndex: 'trigger_type',
      key: 'trigger_type',
      width: 100,
      render: (t: string) =>
        TRIGGER_TYPE_MAP[t] ? (
          <Tag color={TRIGGER_TYPE_MAP[t].color}>{TRIGGER_TYPE_MAP[t].label}</Tag>
        ) : (
          t || '-'
        ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (s: string) => <StatusTag status={s} map={RUN_STATUS_MAP} />,
    },
    {
      title: '耗时',
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 100,
      align: 'right' as const,
      render: (ms: number) => formatDuration(ms),
    },
    {
      title: 'Token',
      dataIndex: 'total_tokens',
      key: 'total_tokens',
      width: 110,
      align: 'right' as const,
      render: (v: number) => formatNumber(v),
    },
    {
      title: '成本',
      dataIndex: 'cost',
      key: 'cost',
      width: 100,
      align: 'right' as const,
      render: (v: number | string) => `${formatNumber(v)} $`,
    },
    {
      title: '开始时间',
      key: 'started_at',
      width: 180,
      render: (_: unknown, r: AIWorkflowRun) => formatDateTime(r.started_at || r.created_at),
    },
    {
      title: '操作',
      key: 'action',
      width: 110,
      fixed: 'right' as const,
      render: (_: unknown, r: AIWorkflowRun) => (
        <Button
          type="link"
          size="small"
          icon={<EyeOutlined />}
          onClick={() => {
            setCurrent(r);
            setDetailOpen(true);
          }}
        >
          查看详情
        </Button>
      ),
    },
  ];

  /** 复制 JSON 到剪贴板 */
  const copyJSON = (text?: string) => {
    if (!text) {
      message.warning('暂无内容可复制');
      return;
    }
    navigator.clipboard
      .writeText(text)
      .then(() => message.success('已复制到剪贴板'))
      .catch(() => message.error('复制失败'));
  };

  // 详情 Modal 预格式化内容
  const detailInput = useMemo(() => {
    const parsed = safeParseJSON(current?.input);
    return typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2);
  }, [current]);

  const detailOutput = useMemo(() => {
    // 失败时优先展示 error 字段
    if (current?.status === 'failed' && current.error) return current.error;
    const parsed = safeParseJSON(current?.output);
    return typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2);
  }, [current]);

  return (
    <PageContainer title="AI 工作流历史记录" breadcrumb={{}}>
      {/* 顶部统计卡 */}
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={24} sm={12} md={12} xl={6}>
          <Card className="cbp-enterprise-card" bordered={false} styles={{ body: { padding: 20 } }}>
            <Space align="start" size={12} style={{ width: '100%' }}>
              <span
                className="cbp-icon-tile"
                style={{ background: 'rgba(82,196,26,0.12)', color: '#52c41a' }}
              >
                <CheckCircleOutlined />
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Statistic title="总执行次数" value={stats.totalCount} />
              </div>
            </Space>
          </Card>
        </Col>
        <Col xs={24} sm={12} md={12} xl={6}>
          <Card className="cbp-enterprise-card" bordered={false} styles={{ body: { padding: 20 } }}>
            <Space align="start" size={12} style={{ width: '100%' }}>
              <span
                className="cbp-icon-tile"
                style={{ background: 'rgba(22,119,255,0.12)', color: '#1677ff' }}
              >
                <ThunderboltOutlined />
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Statistic
                  title="成功次数"
                  value={stats.successCount}
                  suffix={
                    <Text type="secondary" style={{ fontSize: 14 }}>
                      {stats.successRate.toFixed(1)}%
                    </Text>
                  }
                />
              </div>
            </Space>
          </Card>
        </Col>
        <Col xs={24} sm={12} md={12} xl={6}>
          <Card className="cbp-enterprise-card" bordered={false} styles={{ body: { padding: 20 } }}>
            <Space align="start" size={12} style={{ width: '100%' }}>
              <span
                className="cbp-icon-tile"
                style={{ background: 'rgba(250,140,22,0.12)', color: '#fa8c16' }}
              >
                <ClockCircleOutlined />
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Statistic title="平均耗时" value={Math.round(stats.avgDuration)} suffix="ms" />
              </div>
            </Space>
          </Card>
        </Col>
        <Col xs={24} sm={12} md={12} xl={6}>
          <Card className="cbp-enterprise-card" bordered={false} styles={{ body: { padding: 20 } }}>
            <Space align="start" size={12} style={{ width: '100%' }}>
              <span
                className="cbp-icon-tile"
                style={{ background: 'rgba(114,46,209,0.12)', color: '#722ed1' }}
              >
                <FireOutlined />
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Statistic title="累计 Token 消耗" value={stats.totalTokens} />
              </div>
            </Space>
          </Card>
        </Col>
      </Row>

      {/* 筛选区 */}
      <Card
        className="cbp-enterprise-card"
        bordered={false}
        style={{ marginBottom: 16 }}
        styles={{ body: { padding: 20 } }}
      >
        <Form form={form} layout="inline" onFinish={onSearch}>
          <Space wrap size={[12, 12]} style={{ width: '100%' }}>
            <Form.Item name="workflow_code" label="工作流代码">
              <Input placeholder="如 wf_product_analysis" allowClear style={{ width: 220 }} />
            </Form.Item>
            <Form.Item name="status" label="状态" initialValue="all">
              <Select
                style={{ width: 140 }}
                options={[
                  { value: 'all', label: '全部' },
                  { value: 'success', label: '成功' },
                  { value: 'failed', label: '失败' },
                  { value: 'running', label: '运行中' },
                  { value: 'timeout', label: '超时' },
                ]}
              />
            </Form.Item>
            <Form.Item name="trigger_type" label="触发类型" initialValue="all">
              <Select
                style={{ width: 140 }}
                options={[
                  { value: 'all', label: '全部' },
                  { value: 'manual', label: '手动' },
                  { value: 'scheduled', label: '定时' },
                  { value: 'event', label: '事件' },
                ]}
              />
            </Form.Item>
            <Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" icon={<ReloadOutlined />}>
                  查询
                </Button>
                <Button onClick={onReset}>重置</Button>
              </Space>
            </Form.Item>
          </Space>
        </Form>
      </Card>

      {/* 趋势图 */}
      <Card
        className="cbp-enterprise-card"
        bordered={false}
        title="近 30 天执行趋势"
        style={{ marginBottom: 16 }}
        styles={{ body: { padding: 20 } }}
      >
        {trendOption ? (
          <ReactECharts option={trendOption} style={{ height: 280 }} />
        ) : (
          <Empty description="暂无趋势数据" />
        )}
      </Card>

      {/* 历史记录表格 */}
      <Card className="cbp-enterprise-card" bordered={false} styles={{ body: { padding: 20 } }}>
        <Spin spinning={loading}>
          <Table<AIWorkflowRun>
            rowKey="id"
            columns={columns}
            dataSource={displayRuns}
            scroll={{ x: 1200 }}
            pagination={{
              current: page,
              pageSize,
              total,
              showSizeChanger: true,
              showTotal: (t) => `共 ${t} 条`,
              onChange: (p, ps) => {
                setPage(p);
                setPageSize(ps);
              },
            }}
          />
        </Spin>
      </Card>

      {/* 详情 Modal */}
      <Modal
        title="运行详情"
        open={detailOpen}
        onCancel={() => setDetailOpen(false)}
        width={820}
        footer={[
          <Button
            key="copy"
            icon={<CopyOutlined />}
            onClick={() => copyJSON(current?.output || current?.error)}
          >
            复制 JSON
          </Button>,
          <Button key="close" type="primary" onClick={() => setDetailOpen(false)}>
            关闭
          </Button>,
        ]}
      >
        {current && (
          <>
            <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="工作流代码">{current.workflow_code}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <StatusTag status={current.status} map={RUN_STATUS_MAP} />
              </Descriptions.Item>
              <Descriptions.Item label="触发类型">
                {TRIGGER_TYPE_MAP[current.trigger_type] ? (
                  <Tag color={TRIGGER_TYPE_MAP[current.trigger_type].color}>
                    {TRIGGER_TYPE_MAP[current.trigger_type].label}
                  </Tag>
                ) : (
                  current.trigger_type || '-'
                )}
              </Descriptions.Item>
              <Descriptions.Item label="耗时">{formatDuration(current.duration_ms)}</Descriptions.Item>
              <Descriptions.Item label="Token">{formatNumber(current.total_tokens)}</Descriptions.Item>
              <Descriptions.Item label="成本">{formatNumber(current.cost)} $</Descriptions.Item>
              <Descriptions.Item label="开始时间">
                {formatDateTime(current.started_at || current.created_at)}
              </Descriptions.Item>
              <Descriptions.Item label="完成时间">
                {formatDateTime(current.completed_at)}
              </Descriptions.Item>
            </Descriptions>
            <Title level={5} style={{ marginTop: 8, marginBottom: 8 }}>
              输入参数
            </Title>
            <pre className="cbp-code-block">{detailInput}</pre>
            <Title level={5} style={{ marginTop: 16, marginBottom: 8 }}>
              {current.status === 'failed' ? '错误信息' : '输出结果'}
            </Title>
            <pre className="cbp-code-block">{detailOutput}</pre>
          </>
        )}
      </Modal>
    </PageContainer>
  );
};

export default AIWorkflowRuns;
