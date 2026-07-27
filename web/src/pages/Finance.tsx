/**
 * 对账利润:企业级 Tab(对账单列表 / 利润分析驾驶舱)
 */
import React, { useEffect, useRef, useState } from 'react';
import {
  Tabs,
  Card,
  Tag,
  Row,
  Col,
  Typography,
  Spin,
  Empty,
  Table,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Space,
  message,
} from 'antd';
import {
  AuditOutlined,
  PayCircleOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { listBills, matchBill, payBill, getProfitSummary, getProfitBySku } from '@/api/finance';
import { getProfitStats } from '@/api/dashboard';
import { useAuthStore } from '@/store/auth';
import { BILL_STATUS_MAP } from '@/utils/constants';
import { formatCNY, formatPercent, formatDateTime, formatNumber } from '@/utils/format';
import type { FinanceBill, BillStatus, ProfitSummary, ProfitBySku, ProfitStats } from '@/types/api';

const { Text } = Typography;

const BILL_STATUS_OPTIONS = Object.entries(BILL_STATUS_MAP).map(([value, { label }]) => ({
  value: value as BillStatus,
  label,
}));

const MATCHABLE_STATUSES: BillStatus[] = ['draft', 'matching', 'disputed'];

const Finance: React.FC = () => {
  const [activeTab, setActiveTab] = useState('bills');
  return (
    <PageContainer title="对账利润" breadcrumb={{}}>
      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        items={[
          { key: 'bills', label: '对账单', children: <BillsTab /> },
          { key: 'profit', label: '利润分析', children: <ProfitTab /> },
        ]}
      />
    </PageContainer>
  );
};

const BillsTab: React.FC = () => {
  const actionRef = useRef<ActionType>(null);
  const hasRole = useAuthStore((s) => s.hasRole);
  const canWrite = hasRole('admin', 'staff');

  const [matchingId, setMatchingId] = useState<number | null>(null);
  const [payOpen, setPayOpen] = useState(false);
  const [payRecord, setPayRecord] = useState<FinanceBill | null>(null);
  const [paySubmitting, setPaySubmitting] = useState(false);
  const [payForm] = Form.useForm();

  const handleMatch = (record: FinanceBill) => {
    Modal.confirm({
      title: '确认对账',
      icon: <AuditOutlined style={{ color: '#1677ff' }} />,
      content: (
        <div>
          <div>账单号：{record.bill_no}</div>
          <div>应付金额：{formatCNY(record.payable_amount)}</div>
          <div style={{ marginTop: 8 }}>
            <Text type="secondary">确认后将账单标记为已对平</Text>
          </div>
        </div>
      ),
      okText: '确认对账',
      cancelText: '取消',
      onOk: async () => {
        setMatchingId(record.id);
        try {
          await matchBill(record.id);
          message.success(`账单 ${record.bill_no} 对账成功`);
          actionRef.current?.reload();
        } catch {
          // 错误已由拦截器提示
        } finally {
          setMatchingId(null);
        }
      },
    });
  };

  const openPay = (record: FinanceBill) => {
    setPayRecord(record);
    payForm.resetFields();
    const remain = Math.max(0, Number(record.payable_amount) - Number(record.paid_amount || 0));
    payForm.setFieldsValue({
      paid_amount: remain > 0 ? remain : Number(record.payable_amount),
      remark: '',
    });
    setPayOpen(true);
  };

  const handlePay = async () => {
    if (!payRecord) return;
    const values = await payForm.validateFields();
    setPaySubmitting(true);
    try {
      await payBill(payRecord.id, {
        paid_amount: values.paid_amount,
        remark: values.remark,
      });
      message.success(`账单 ${payRecord.bill_no} 付款成功`);
      setPayOpen(false);
      setPayRecord(null);
      payForm.resetFields();
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setPaySubmitting(false);
    }
  };

  const columns: ProColumns<FinanceBill>[] = [
    { title: '账单搜索(单号)', dataIndex: 'keyword', hideInTable: true },
    {
      title: '账单号',
      dataIndex: 'bill_no',
      width: 180,
      copyable: true,
    },
    {
      title: '采购单号',
      dataIndex: 'order_no',
      width: 180,
      search: false,
      copyable: true,
    },
    {
      title: '供应商',
      dataIndex: 'supplier_name',
      width: 160,
      search: false,
      ellipsis: true,
    },
    {
      title: '应付金额',
      dataIndex: 'payable_amount',
      width: 130,
      align: 'right',
      search: false,
      render: (_, r) => formatCNY(r.payable_amount),
    },
    {
      title: '已付金额',
      dataIndex: 'paid_amount',
      width: 130,
      align: 'right',
      search: false,
      render: (_, r) => formatCNY(r.paid_amount),
    },
    {
      title: '差异金额',
      dataIndex: 'diff_amount',
      width: 130,
      align: 'right',
      search: false,
      sorter: true,
      render: (_, r) => {
        if (Number(r.diff_amount) === 0) {
          return <Text type="secondary">¥0.00</Text>;
        }
        return (
          <Text type="danger" strong style={{ background: '#fff1f0', padding: '2px 8px', borderRadius: 4 }}>
            {formatCNY(r.diff_amount)}
          </Text>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      valueType: 'select',
      fieldProps: { options: BILL_STATUS_OPTIONS },
      render: (_, r) => <StatusTag status={r.status} map={BILL_STATUS_MAP} />,
    },
    {
      title: '对账时间',
      dataIndex: 'matched_at',
      width: 170,
      search: false,
      render: (_, r) => (r.matched_at ? formatDateTime(r.matched_at) : '-'),
    },
    {
      title: '付款时间',
      dataIndex: 'paid_at',
      width: 170,
      search: false,
      render: (_, r) => (r.paid_at ? formatDateTime(r.paid_at) : '-'),
    },
    {
      title: '操作',
      valueType: 'option',
      width: 140,
      fixed: 'right',
      render: (_, r) => {
        if (!canWrite) return [];
        const actions: React.ReactNode[] = [];
        if (MATCHABLE_STATUSES.includes(r.status)) {
          actions.push(
            <Button
              key="match"
              type="link"
              size="small"
              icon={<AuditOutlined />}
              loading={matchingId === r.id}
              onClick={() => handleMatch(r)}
            >
              对账
            </Button>,
          );
        }
        if (r.status === 'matched') {
          actions.push(
            <Button
              key="pay"
              type="link"
              size="small"
              icon={<PayCircleOutlined />}
              onClick={() => openPay(r)}
            >
              付款
            </Button>,
          );
        }
        return actions;
      },
    },
  ];

  return (
    <>
      <ProTable<FinanceBill>
        rowKey="id"
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1640 }}
        search={{ labelWidth: 100 }}
        cardBordered
        request={async (params) => {
          const res = await listBills({
            page: params.current,
            page_size: params.pageSize,
            keyword: params.keyword,
            status: params.status as BillStatus | undefined,
          });
          return { data: res.list, total: res.total, success: true };
        }}
        pagination={{ pageSize: 10, showSizeChanger: true }}
        options={{ density: false, fullScreen: true, reload: true }}
        headerTitle="对账单列表"
      />

      <Modal
        title={
          <Space>
            <PayCircleOutlined style={{ color: '#1677ff' }} />
            <span>账单付款</span>
          </Space>
        }
        open={payOpen}
        onCancel={() => {
          setPayOpen(false);
          setPayRecord(null);
          payForm.resetFields();
        }}
        onOk={handlePay}
        confirmLoading={paySubmitting}
        destroyOnClose
        okText="确认付款"
        cancelText="取消"
        width={480}
      >
        {payRecord && (
          <div style={{ marginBottom: 16, padding: '12px 16px', background: '#fafafa', borderRadius: 8 }}>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <div>
                <Text type="secondary">账单号：</Text>
                <Text strong copyable>
                  {payRecord.bill_no}
                </Text>
              </div>
              <div>
                <Text type="secondary">供应商：</Text>
                <Text>{payRecord.supplier_name || '-'}</Text>
              </div>
              <div>
                <Text type="secondary">应付金额：</Text>
                <Text strong style={{ color: '#1677ff' }}>
                  {formatCNY(payRecord.payable_amount)}
                </Text>
              </div>
              <div>
                <Text type="secondary">已付金额：</Text>
                <Text>{formatCNY(payRecord.paid_amount)}</Text>
              </div>
            </Space>
          </div>
        )}
        <Form form={payForm} layout="vertical">
          <Form.Item
            name="paid_amount"
            label="本次付款金额"
            rules={[
              { required: true, message: '请输入付款金额' },
              { type: 'number', min: 0.01, message: '付款金额须大于 0' },
            ]}
          >
            <InputNumber
              min={0.01}
              precision={2}
              style={{ width: '100%' }}
              placeholder="请输入付款金额"
              addonBefore="¥"
            />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={3} placeholder="可选，填写付款说明" maxLength={200} showCount />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

const ProfitTab: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [summary, setSummary] = useState<ProfitSummary | null>(null);
  const [bySku, setBySku] = useState<ProfitBySku[]>([]);
  const [profitStats, setProfitStats] = useState<ProfitStats | null>(null);

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const [s, list, ps] = await Promise.all([
          getProfitSummary(),
          getProfitBySku(),
          getProfitStats(),
        ]);
        setSummary(s);
        setBySku(list?.list || []);
        setProfitStats(ps);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const monthData = [...(profitStats?.by_month || [])].reverse();

  const monthOption: EChartsOption = {
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params: any) => {
        const m = params[0]?.name || '';
        let s = m;
        params.forEach((p: any) => {
          s += `<br/>${p.marker} ${p.seriesName}: ¥${Number(p.value).toLocaleString('zh-CN', {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          })}`;
        });
        return s;
      },
    },
    legend: { data: ['收入', '净利润'], top: 4, icon: 'roundRect', itemWidth: 12, itemHeight: 8 },
    grid: { left: 60, right: 30, top: 50, bottom: 40 },
    xAxis: {
      type: 'category',
      data: monthData.map((m) => m.month),
      axisLabel: { color: 'rgba(0,0,0,0.45)', fontSize: 11 },
      axisLine: { lineStyle: { color: '#eef0f4' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      name: '金额 (¥)',
      axisLabel: { color: 'rgba(0,0,0,0.45)', fontSize: 11 },
      splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
    },
    series: [
      {
        name: '收入',
        type: 'bar',
        barWidth: 14,
        itemStyle: { color: '#1677ff', borderRadius: [4, 4, 0, 0] },
        data: monthData.map((m) => Number(m.revenue)),
      },
      {
        name: '净利润',
        type: 'bar',
        barWidth: 14,
        itemStyle: { color: '#52c41a', borderRadius: [4, 4, 0, 0] },
        data: monthData.map((m) => Number(m.net_profit)),
      },
    ],
  };

  const costBreakdown = profitStats?.cost_breakdown;
  const costPieData = costBreakdown
    ? [
        { name: '采购成本', value: Number(costBreakdown.goods_cost) },
        { name: '物流成本', value: Number(costBreakdown.freight_cost) },
        { name: '平台费用', value: Number(costBreakdown.platform_fee) },
        { name: '广告成本', value: Number(costBreakdown.ad_cost) },
        { name: '税费', value: Number(costBreakdown.tax_cost) },
        { name: '退款损失', value: Number(costBreakdown.refund_cost) },
        { name: '其他', value: Number(costBreakdown.other_cost) },
      ].filter((d) => d.value > 0)
    : [];

  const costPieOption: EChartsOption = {
    tooltip: {
      trigger: 'item',
      formatter: (p: any) =>
        `${p.name}<br/>¥${Number(p.value).toLocaleString('zh-CN', { minimumFractionDigits: 2 })} (${p.percent}%)`,
    },
    legend: { orient: 'vertical', left: 'left', top: 'middle', textStyle: { fontSize: 12 } },
    series: [
      {
        name: '成本结构',
        type: 'pie',
        radius: ['45%', '70%'],
        center: ['60%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
        label: {
          show: true,
          formatter: '{b}: {d}%',
          fontSize: 11,
        },
        data: costPieData,
        color: ['#1677ff', '#52c41a', '#fa8c16', '#722ED1', '#13c2c2', '#fa541c', '#8c8c8c'],
      },
    ],
  };

  const categoryOption: EChartsOption = {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    legend: { data: ['收入', '利润'], top: 4, icon: 'roundRect', itemWidth: 12, itemHeight: 8 },
    grid: { left: 60, right: 30, top: 50, bottom: 40 },
    xAxis: {
      type: 'category',
      data: bySku.map((s) => s.sku),
      axisLabel: { color: 'rgba(0,0,0,0.45)', rotate: 15 },
      axisLine: { lineStyle: { color: '#eef0f4' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      name: '金额 (¥)',
      axisLabel: { color: 'rgba(0,0,0,0.45)' },
      splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
    },
    series: [
      {
        name: '收入',
        type: 'bar',
        itemStyle: { color: '#2F54EB', borderRadius: [4, 4, 0, 0] },
        data: bySku.map((s) => Number(s.revenue)),
      },
      {
        name: '利润',
        type: 'bar',
        itemStyle: { color: '#52c41a', borderRadius: [4, 4, 0, 0] },
        data: bySku.map((s) => Number(s.net_profit)),
      },
    ],
  };

  const marginOption: EChartsOption = {
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 60, right: 30, top: 30, bottom: 40 },
    xAxis: {
      type: 'category',
      data: bySku.map((s) => s.sku),
      axisLabel: { color: 'rgba(0,0,0,0.45)', rotate: 15 },
      axisLine: { lineStyle: { color: '#eef0f4' } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      name: '毛利率',
      axisLabel: { formatter: '{value}%' },
      splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
    },
    series: [
      {
        name: '毛利率',
        type: 'bar',
        itemStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: '#722ED1' },
              { offset: 1, color: '#2F54EB' },
            ],
          },
          borderRadius: [4, 4, 0, 0],
        },
        data: bySku.map((s) => +Number(s.margin_rate).toFixed(1)),
      },
    ],
  };

  const skuColumns = [
    {
      title: 'SKU',
      dataIndex: 'sku',
      render: (v: string) => <Text strong>{v}</Text>,
    },
    {
      title: '订单数',
      dataIndex: 'order_count',
      align: 'right' as const,
      render: (v: number) => formatNumber(v),
    },
    {
      title: '收入',
      dataIndex: 'revenue',
      align: 'right' as const,
      render: (v: number | string) => formatCNY(Number(v)),
    },
    {
      title: '成本合计',
      align: 'right' as const,
      render: (_: unknown, s: ProfitBySku) =>
        formatCNY(
          Number(s.goods_cost) + Number(s.freight_cost) + Number(s.platform_fee) + Number(s.ad_cost),
        ),
    },
    {
      title: '净利润',
      dataIndex: 'net_profit',
      align: 'right' as const,
      render: (v: number | string) => (
        <Text strong style={{ color: '#52c41a' }}>
          {formatCNY(Number(v))}
        </Text>
      ),
    },
    {
      title: '毛利率',
      dataIndex: 'margin_rate',
      align: 'right' as const,
      render: (v: number | string) => {
        const margin = Number(v);
        return (
          <Tag color={margin >= 35 ? 'green' : margin >= 20 ? 'gold' : 'red'} style={{ borderRadius: 6 }}>
            {margin.toFixed(1)}%
          </Tag>
        );
      },
    },
  ];

  return (
    <Spin spinning={loading}>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">总收入</div>
            <div className="value" style={{ color: '#1677ff' }}>
              {formatCNY(Number(summary?.total_revenue ?? 0))}
            </div>
            <div className="hint">累计销售回款口径</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">总利润</div>
            <div className="value" style={{ color: '#52c41a' }}>
              {formatCNY(Number(summary?.total_net_profit ?? 0))}
            </div>
            <div className="hint">扣除全链路成本后</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">平均毛利率</div>
            <div className="value" style={{ color: '#722ed1' }}>
              {formatPercent(Number(summary?.avg_margin_rate ?? 0) / 100, 1)}
            </div>
            <div className="hint">按 SKU 加权均值</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">订单量</div>
            <div className="value">{formatNumber(summary?.order_count)}</div>
            <div className="hint">利润统计周期内</div>
          </div>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
            <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
              月度收入与利润(近 12 个月)
            </div>
            {monthData.length === 0 ? (
              <Empty description="暂无月度数据" style={{ padding: '40px 0' }} />
            ) : (
              <ReactECharts option={monthOption} style={{ height: 320 }} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
            <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
              成本结构分解
            </div>
            {costPieData.length === 0 ? (
              <Empty description="暂无成本数据" style={{ padding: '40px 0' }} />
            ) : (
              <ReactECharts option={costPieOption} style={{ height: 320 }} />
            )}
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={12}>
          <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }}>
            <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
              各 SKU 收入与利润
            </div>
            {bySku.length === 0 ? (
              <Empty description="暂无 SKU 利润数据" style={{ padding: '40px 0' }} />
            ) : (
              <ReactECharts option={categoryOption} style={{ height: 320 }} />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }}>
            <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
              各 SKU 毛利率
            </div>
            {bySku.length === 0 ? (
              <Empty description="暂无毛利率数据" style={{ padding: '40px 0' }} />
            ) : (
              <ReactECharts option={marginOption} style={{ height: 320 }} />
            )}
          </Card>
        </Col>
      </Row>

      <Card
        title={<div className="cbp-section-title">按 SKU 利润明细</div>}
        bordered={false}
        style={{ marginTop: 16 }}
        styles={{ body: { paddingTop: 8 } }}
      >
        <Table
          rowKey="sku"
          size="middle"
          columns={skuColumns}
          dataSource={bySku}
          pagination={{ pageSize: 8, showSizeChanger: false }}
          locale={{ emptyText: <Empty description="暂无明细数据" /> }}
        />
      </Card>
    </Spin>
  );
};

export default Finance;
