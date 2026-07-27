/**
 * 工作台:企业级数据驾驶舱
 * - 顶部 8 个 KPI 指标卡(左侧色条 + 半透明图标 + 同比指标)
 * - 销售与利润双轴趋势(30 天)
 * - 品类销售占比环形图
 * - 成本结构分解
 * - AI 场景使用分布
 * - 选品阶段漏斗 / 采购状态分布 / 库存仓库分布
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Row, Col, Card, Spin, Typography, Space, Tag, Tooltip, Empty, Skeleton, Table, Input, Button, Alert, message } from 'antd';
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  DollarCircleOutlined,
  RiseOutlined,
  ShoppingOutlined,
  WarningOutlined,
  AuditOutlined,
  RobotOutlined,
  AppstoreOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import PageContainer from '@/components/PageContainer';
import {
  getOverview,
  getSalesTrend,
  getCategoryShare,
  getProductStats,
  getProfitStats,
  getAIStats,
  type AIStatsResp,
} from '@/api/dashboard';
import { formatCNY, formatNumber, formatPercent, formatChartDate } from '@/utils/format';
import { runAIWorkflow } from '@/api/ai';
import type {
  DashboardOverview,
  SalesTrendPoint,
  CategorySharePoint,
  ProductStats,
  ProfitStats,
} from '@/types/api';

const { Text } = Typography;

/** AI 工作流场景中文名 */
const AI_SCENE_NAME: Record<string, string> = {
  wf_product_analysis: '选品分析',
  wf_purchase_assistant: '采购助手',
  wf_customer_service: '智能客服',
  wf_data_analysis: '数据分析',
  wf_listing_generator: 'Listing 生成',
  wf_content_generation: '内容生成',
};

/** 品类渐变配色（用于玫瑰图扇区填充） */
const CATEGORY_GRADIENT: Record<string, { from: string; to: string }> = {
  personal_care: { from: '#69b1ff', to: '#1677ff' },
  beauty_device: { from: '#b37feb', to: '#722ed1' },
  body_shaping: { from: '#ff85c0', to: '#eb2f96' },
  accessories: { from: '#ffc069', to: '#fa8c16' },
};

const Dashboard: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [overview, setOverview] = useState<DashboardOverview | null>(null);
  const [trend, setTrend] = useState<SalesTrendPoint[]>([]);
  const [share, setShare] = useState<CategorySharePoint[]>([]);
  const [productStats, setProductStats] = useState<ProductStats | null>(null);
  const [profitStats, setProfitStats] = useState<ProfitStats | null>(null);
  const [aiStats, setAIStats] = useState<AIStatsResp | null>(null);
  const [aiQuestion, setAiQuestion] = useState('');
  const [aiDataAnalyzing, setAiDataAnalyzing] = useState(false);
  const [aiDataResult, setAiDataResult] = useState<{
    sql: string;
    result: Array<Record<string, any>>;
    insight: string;
  } | null>(null);

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const [ov, tr, sh, ps, pr, ai] = await Promise.all([
          getOverview(),
          getSalesTrend(30),
          getCategoryShare(),
          getProductStats(),
          getProfitStats(),
          getAIStats(),
        ]);
        setOverview(ov);
        setTrend(tr);
        setShare(sh);
        setProductStats(ps);
        setProfitStats(pr);
        setAIStats(ai);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  // AI 数据分析助手:调用 wf_data_analysis(id=4),返回 SQL / 结果集 / 业务洞察
  const handleAiDataAnalysis = async () => {
    if (!aiQuestion.trim()) {
      message.warning('请输入分析问题');
      return;
    }
    setAiDataAnalyzing(true);
    try {
      const result = await runAIWorkflow(4, { input: { question: aiQuestion } });
      const parsed = JSON.parse(result.output);
      const inner = parsed.parsed || parsed;
      setAiDataResult({
        sql: inner.sql || '',
        result: Array.isArray(inner.result) ? inner.result : [],
        insight: inner.insight || '分析完成',
      });
      message.success('AI 数据分析完成');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'AI 分析失败');
    } finally {
      setAiDataAnalyzing(false);
    }
  };

  // ===== 图表配置 =====

  // 销售与利润双轴趋势
  const trendOption: EChartsOption = useMemo(() => ({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255,255,255,0.98)',
      borderColor: '#eef0f4',
      borderWidth: 1,
      textStyle: { color: 'rgba(0,0,0,0.85)', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.10); border-radius: 10px;',
    },
    legend: {
      data: ['销售额', '净利润', '订单量'],
      top: 4,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 12,
      textStyle: { color: 'rgba(0,0,0,0.55)', fontSize: 12 },
    },
    grid: { left: 56, right: 56, top: 50, bottom: 40 },
    xAxis: {
      type: 'category',
      boundaryGap: true,
      data: trend.map((t) => t.date.substring(5)),
      axisLine: { lineStyle: { color: '#eef0f4' } },
      axisLabel: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        name: '金额 (¥)',
        nameTextStyle: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
        splitLine: { lineStyle: { color: '#f3f4f7', type: 'dashed', opacity: 0.7 } },
        axisLabel: {
          color: 'rgba(0,0,0,0.42)',
          fontSize: 11,
          formatter: (v: number) => (v >= 10000 ? `${(v / 10000).toFixed(1)}万` : `${v}`),
        },
      },
      {
        type: 'value',
        name: '订单量',
        nameTextStyle: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
        splitLine: { show: false },
        axisLabel: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
      },
    ],
    series: [
      {
        name: '销售额',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 5,
        showSymbol: false,
        itemStyle: { color: '#1677ff' },
        lineStyle: { width: 2.5 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(22,119,255,0.32)' },
              { offset: 1, color: 'rgba(22,119,255,0.02)' },
            ],
          },
        },
        data: trend.map((t) => Number(t.sales)),
      },
      {
        name: '净利润',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 5,
        showSymbol: false,
        itemStyle: { color: '#722ed1' },
        lineStyle: { width: 2 },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(114,46,209,0.26)' },
              { offset: 1, color: 'rgba(114,46,209,0.02)' },
            ],
          },
        },
        data: trend.map((t) => Number(t.net_profit)),
      },
      {
        name: '订单量',
        type: 'bar',
        yAxisIndex: 1,
        barWidth: 10,
        itemStyle: {
          borderRadius: [4, 4, 0, 0],
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(250,140,22,0.85)' },
              { offset: 1, color: 'rgba(250,140,22,0.30)' },
            ],
          },
        },
        data: trend.map((t) => Number(t.orders)),
      },
    ],
  }), [trend]);

  // 品类销售占比环形图
  const shareOption: EChartsOption = useMemo(() => ({
    tooltip: {
      trigger: 'item',
      formatter: (params: unknown) => {
        const p = params as { name: string; value: number; percent: number };
        return `${p.name}<br/>金额: ¥${formatNumber(Math.round(p.value))}<br/>占比: ${p.percent}%`;
      },
      backgroundColor: 'rgba(255,255,255,0.98)',
      borderColor: '#eef0f4',
      borderWidth: 1,
      textStyle: { color: 'rgba(0,0,0,0.85)', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.10); border-radius: 10px;',
    },
    legend: {
      orient: 'vertical',
      left: 'left',
      top: 'middle',
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 12,
      textStyle: { color: 'rgba(0,0,0,0.65)', fontSize: 12 },
    },
    series: [
      {
        name: '品类销售占比',
        type: 'pie',
        radius: ['52%', '72%'],
        center: ['62%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
        label: {
          show: true,
          position: 'center',
          formatter: () => {
            const total = share.reduce((s, x) => s + Number(x.sales), 0);
            return `{a|总销售}\n{b|¥${formatNumber(Math.round(total))}}`;
          },
          rich: {
            a: { fontSize: 12, color: 'rgba(0,0,0,0.45)', padding: [4, 0, 4, 0] },
            b: { fontSize: 18, fontWeight: 600, color: 'rgba(0,0,0,0.85)' },
          },
        },
        emphasis: {
          label: { show: true, fontSize: 14, fontWeight: 'bold' },
          scaleSize: 6,
          itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.10)' },
        },
        data: share.map((s) => {
          const g = CATEGORY_GRADIENT[s.category] ?? { from: '#69b1ff', to: '#1677ff' };
          return {
            name: s.category_name,
            value: Number(s.sales),
            itemStyle: {
              color: {
                type: 'linear',
                x: 0, y: 0, x2: 1, y2: 1,
                colorStops: [
                  { offset: 0, color: g.from },
                  { offset: 1, color: g.to },
                ],
              },
            },
          };
        }),
      },
    ],
  }), [share]);

  // 成本结构分解(横向条形)
  const costOption: EChartsOption = useMemo(() => {
    if (!profitStats?.cost_breakdown) return {};
    const cost = profitStats.cost_breakdown;
    const costNames: Record<string, string> = {
      goods_cost: '货物成本',
      freight_cost: '头程运费',
      platform_fee: '平台佣金',
      ad_cost: '广告费',
      tax_cost: '税费',
      refund_cost: '退款损失',
      other_cost: '其他',
    };
    const data = Object.entries(cost).map(([k, v]) => ({ name: costNames[k] ?? k, value: Number(v) }));
    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as Array<{ name: string; value: number }>;
          return arr.map((p) => `${p.name}: ¥${formatNumber(p.value)}`).join('<br/>');
        },
      },
      grid: { left: 80, right: 30, top: 20, bottom: 30 },
      xAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        axisLabel: {
          color: 'rgba(0,0,0,0.45)',
          fontSize: 11,
          formatter: (v: number) => (v >= 10000 ? `${(v / 10000).toFixed(1)}万` : `${v}`),
        },
      },
      yAxis: {
        type: 'category',
        data: data.map((d) => d.name),
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
        axisLabel: { color: 'rgba(0,0,0,0.65)', fontSize: 12 },
      },
      series: [
        {
          type: 'bar',
          barWidth: 14,
          itemStyle: {
            borderRadius: [0, 4, 4, 0],
            color: {
              type: 'linear',
              x: 0, y: 0, x2: 1, y2: 0,
              colorStops: [
                { offset: 0, color: '#4096ff' },
                { offset: 1, color: '#1677ff' },
              ],
            },
          },
          label: {
            show: true,
            position: 'right',
            color: 'rgba(0,0,0,0.65)',
            fontSize: 11,
            formatter: (p: any) => `¥${formatNumber(Math.round(Number(p.value)))}`,
          },
          data: data.map((d) => d.value),
        },
      ],
    };
  }, [profitStats]);

  // AI 场景使用分布(条形)
  const aiSceneOption: EChartsOption = useMemo(() => {
    if (!aiStats) return {};
    const data = [...(aiStats.by_scene || [])]
      .sort((a, b) => b.count - a.count)
      .slice(0, 6);
    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        formatter: (params: unknown) => {
          const arr = params as Array<{ dataIndex: number; value: number }>;
          const item = data[arr[0]?.dataIndex];
          if (!item) return '';
          return [
            `<b>${AI_SCENE_NAME[item.workflow_code] ?? item.workflow_code}</b>`,
            `执行次数: ${item.count}`,
            `成功: ${item.success_count} (${item.count > 0 ? ((item.success_count / item.count) * 100).toFixed(1) : 0}%)`,
            `Tokens: ${formatNumber(item.tokens)}`,
            `平均耗时: ${Math.round(item.avg_duration_ms)}ms`,
          ].join('<br/>');
        },
      },
      grid: { left: 90, right: 30, top: 20, bottom: 30 },
      xAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        axisLabel: { color: 'rgba(0,0,0,0.45)', fontSize: 11 },
      },
      yAxis: {
        type: 'category',
        data: data.map((d) => AI_SCENE_NAME[d.workflow_code] ?? d.workflow_code),
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
        axisLabel: { color: 'rgba(0,0,0,0.65)', fontSize: 12 },
      },
      series: [
        {
          type: 'bar',
          barWidth: 14,
          itemStyle: {
            borderRadius: [0, 4, 4, 0],
            color: {
              type: 'linear',
              x: 0, y: 0, x2: 1, y2: 0,
              colorStops: [
                { offset: 0, color: '#9254de' },
                { offset: 1, color: '#722ed1' },
              ],
            },
          },
          label: {
            show: true,
            position: 'right',
            color: 'rgba(0,0,0,0.65)',
            fontSize: 11,
          },
          data: data.map((d) => d.count),
        },
      ],
    };
  }, [aiStats]);

  // 选品阶段漏斗
  const stageOption: EChartsOption = useMemo(() => {
    if (!productStats) return {};
    const stageNames: Record<string, string> = {
      sourcing: '寻源',
      testing: '测试',
      approved: '已通过',
      rejected: '已否决',
      archived: '已归档',
    };
    const data = (productStats.by_stage || [])
      .filter((s) => s.count > 0)
      .map((s) => ({ name: stageNames[s.stage] ?? s.stage, value: s.count }));
    const palette = [
      { from: '#1677ff', to: '#69b1ff' },
      { from: '#4096ff', to: '#91caff' },
      { from: '#52c41a', to: '#95de64' },
      { from: '#faad14', to: '#ffd666' },
      { from: '#ff4d4f', to: '#ff7875' },
      { from: '#8c8c8c', to: '#bfbfbf' },
    ];
    return {
      tooltip: {
        trigger: 'item',
        formatter: '{b}: {c} 个',
        backgroundColor: 'rgba(255,255,255,0.98)',
        borderColor: '#eef0f4',
        borderWidth: 1,
        textStyle: { color: 'rgba(0,0,0,0.85)', fontSize: 12 },
        extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.10); border-radius: 10px;',
      },
      series: [
        {
          type: 'funnel',
          left: '8%',
          top: 16,
          bottom: 16,
          width: '84%',
          min: 0,
          gap: 4,
          label: {
            show: true,
            position: 'inside',
            color: '#fff',
            fontSize: 12,
            fontWeight: 500,
            formatter: '{b}\n{c} 个',
            textShadowBlur: 0,
          },
          labelLine: { show: false },
          itemStyle: { borderColor: '#fff', borderWidth: 2 },
          data: data.map((d, i) => {
            const p = palette[i % palette.length];
            return {
              name: d.name,
              value: d.value,
              itemStyle: {
                color: {
                  type: 'linear',
                  x: 0, y: 0, x2: 1, y2: 0,
                  colorStops: [
                    { offset: 0, color: p.from },
                    { offset: 1, color: p.to },
                  ],
                },
              },
            };
          }),
        },
      ],
    };
  }, [productStats]);

  // 采购状态分布(环形)
  const purchaseStatusOption: EChartsOption = useMemo(() => {
    if (!overview) return {};
    return {
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      series: [
        {
          type: 'pie',
          radius: ['46%', '70%'],
          center: ['50%', '50%'],
          avoidLabelOverlap: true,
          itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
          label: { show: false },
          emphasis: { label: { show: true, fontSize: 13, fontWeight: 'bold' } },
          data: [
            { name: '待处理', value: overview?.pending_purchase_orders ?? 0, itemStyle: { color: '#fa8c16' } },
            { name: '其他已流转', value: Math.max((overview?.purchase_total ?? 0) - (overview?.pending_purchase_orders ?? 0), 0), itemStyle: { color: '#1677ff' } },
          ],
        },
      ],
    };
  }, [overview]);

  // 库存仓库分布(柱状) — 借用 profitStats.by_month 做月度收入/净利润可视化
  const warehouseOption: EChartsOption = useMemo(() => {
    if (!profitStats?.by_month) return {};
    const byMonth = profitStats.by_month;
    const months = [...byMonth].reverse();
    return {
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
      },
      legend: {
        data: ['收入', '净利润'],
        top: 4,
        textStyle: { color: 'rgba(0,0,0,0.65)', fontSize: 12 },
        icon: 'roundRect',
        itemWidth: 12,
        itemHeight: 12,
      },
      grid: { left: 56, right: 30, top: 50, bottom: 40 },
      xAxis: {
        type: 'category',
        data: months.map((m) => m.month),
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisLabel: { color: 'rgba(0,0,0,0.45)', fontSize: 11 },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        axisLabel: {
          color: 'rgba(0,0,0,0.45)',
          fontSize: 11,
          formatter: (v: number) => (v >= 10000 ? `${(v / 10000).toFixed(0)}万` : `${v}`),
        },
      },
      series: [
        {
          name: '收入',
          type: 'bar',
          barWidth: 12,
          itemStyle: { borderRadius: [4, 4, 0, 0], color: '#1677ff' },
          data: months.map((m) => Number(m.revenue)),
        },
        {
          name: '净利润',
          type: 'bar',
          barWidth: 12,
          itemStyle: { borderRadius: [4, 4, 0, 0], color: '#52c41a' },
          data: months.map((m) => Number(m.net_profit)),
        },
      ],
    };
  }, [profitStats]);

  // ===== KPI 卡片配置 =====
  const todaySalesRatio = (() => {
    const cur = Number(overview?.today_sales ?? 0);
    const prev = Number(overview?.yesterday_sales ?? 0);
    if (!cur || !prev) return null;
    return ((cur - prev) / prev) * 100;
  })();

  const kpiCards = [
    {
      key: 'today',
      title: '今日销售额',
      value: formatCNY(Number(overview?.today_sales ?? 0)),
      icon: <DollarCircleOutlined />,
      color: '#1677ff',
      path: '/finance',
      extra: todaySalesRatio === null ? (
        <Text type="secondary" style={{ fontSize: 12 }}>较昨日 -</Text>
      ) : todaySalesRatio >= 0 ? (
        <span className="cbp-kpi-trend-up"><ArrowUpOutlined /> {todaySalesRatio.toFixed(1)}% 较昨日</span>
      ) : (
        <span className="cbp-kpi-trend-down"><ArrowDownOutlined /> {Math.abs(todaySalesRatio).toFixed(1)}% 较昨日</span>
      ),
    },
    {
      key: 'month',
      title: '本月累计销售',
      value: formatCNY(Number(overview?.month_sales ?? 0)),
      icon: <RiseOutlined />,
      color: '#52c41a',
      path: '/finance',
      extra: <Text type="secondary" style={{ fontSize: 12 }}>净利 ¥{formatNumber(Math.round(Number(overview?.month_profit ?? 0)))}</Text>,
    },
    {
      key: 'profit',
      title: '近 30 天净利润',
      value: formatCNY(Number(overview?.net_profit ?? 0)),
      icon: <DollarCircleOutlined />,
      color: '#722ed1',
      path: '/finance',
      extra: <Text type="secondary" style={{ fontSize: 12 }}>毛利率 {formatPercent(Number(overview?.margin_rate ?? 0) / 100, 1)}</Text>,
    },
    {
      key: 'purchase',
      title: '待处理采购单',
      value: formatNumber(overview?.pending_purchase_orders ?? 0),
      icon: <ShoppingOutlined />,
      color: '#fa8c16',
      path: '/purchases',
      extra: <Tag color="orange" style={{ marginTop: 0, borderRadius: 4, fontSize: 11 }}>需跟进</Tag>,
    },
    {
      key: 'inventory',
      title: '库存预警',
      value: formatNumber(overview?.inventory_alerts ?? 0),
      icon: <WarningOutlined />,
      color: '#cf1322',
      path: '/inventory',
      extra: <Tag color="red" style={{ marginTop: 0, borderRadius: 4, fontSize: 11 }}>低于安全库存</Tag>,
    },
    {
      key: 'bills',
      title: '待对账账单',
      value: formatNumber(overview?.bills_pending ?? 0),
      icon: <AuditOutlined />,
      color: '#13c2c2',
      path: '/finance',
      extra: <Text type="secondary" style={{ fontSize: 12 }}>财务待处理</Text>,
    },
    {
      key: 'ai',
      title: '今日 AI 任务',
      value: formatNumber(overview?.ai_runs_today ?? 0),
      icon: <RobotOutlined />,
      color: '#eb2f96',
      path: '/ai-workflows',
      extra: <Text type="secondary" style={{ fontSize: 12 }}>成功率 {formatPercent(Number(overview?.ai_success_rate ?? 0) / 100, 1)}</Text>,
    },
    {
      key: 'product',
      title: '选品池 SKU',
      value: formatNumber(overview?.product_total ?? 0),
      icon: <AppstoreOutlined />,
      color: '#1677ff',
      path: '/products',
      extra: <Text type="secondary" style={{ fontSize: 12 }}>已通过 {overview?.product_approved ?? 0} · 7 日新增 {overview?.new_products_7d ?? 0}</Text>,
    },
  ];

  const chartCardBody = { padding: '16px 20px 20px' };

  return (
    <PageContainer
      title="工作台"
      breadcrumb={{}}
    >
      <Spin spinning={loading} tip="加载经营数据...">
        <div className="cbp-dashboard" style={{ minHeight: 200 }}>
          <div className="cbp-dashboard-banner">
            <div>
              <div className="cbp-dashboard-banner-title">今日经营概览</div>
              <div className="cbp-dashboard-banner-desc">
                覆盖跨境电商「选品 → 采购 → 库存 → 对账」全链路指标，点击卡片可直达业务模块
              </div>
            </div>
            <Space size={8} wrap>
              <Tag color="blue" style={{ borderRadius: 6, margin: 0 }}>实时聚合</Tag>
              <Tag color="geekblue" style={{ borderRadius: 6, margin: 0 }}>近 30 天</Tag>
            </Space>
          </div>

          <Row gutter={[14, 14]}>
            {kpiCards.map((kpi) => (
              <Col xs={12} sm={12} md={8} lg={6} xl={6} xxl={3} key={kpi.key}>
                <Card
                  className="cbp-kpi-card cbp-clickable-card"
                  bordered={false}
                  style={{ '--cbp-kpi-color': kpi.color } as React.CSSProperties}
                  styles={{ body: { padding: '16px 16px 14px 20px' } }}
                  onClick={() => navigate(kpi.path)}
                >
                  <div style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12, marginBottom: 8, letterSpacing: 0.2, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {kpi.title}
                  </div>
                  <div style={{ fontSize: 18, fontWeight: 650, color: 'rgba(0,0,0,0.88)', lineHeight: 1.2, letterSpacing: -0.3, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {loading ? <Skeleton.Input active size="small" style={{ width: 100 }} /> : kpi.value}
                  </div>
                  <div style={{ marginTop: 8, minHeight: 20, fontSize: 12, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{kpi.extra}</div>
                </Card>
              </Col>
            ))}
          </Row>

          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col xs={24} xl={16}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>
                  近 30 天销售与利润趋势
                  <Tooltip title="销售额与净利润共享左轴，订单量为右轴">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 6 }}>说明</Text>
                  </Tooltip>
                </div>
                {trend.length === 0 && !loading ? (
                  <Empty description="暂无销售趋势数据" style={{ padding: '48px 0' }} />
                ) : (
                  <ReactECharts option={trendOption} style={{ height: 320 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={8}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>各品类销售占比</div>
                {share.length === 0 && !loading ? (
                  <Empty description="暂无品类数据" style={{ padding: '48px 0' }} />
                ) : (
                  <ReactECharts option={shareOption} style={{ height: 320 }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col xs={24} xl={12}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>
                  成本结构分解
                  <Tooltip title="近 30 天利润报表成本项汇总">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 6 }}>说明</Text>
                  </Tooltip>
                </div>
                {!profitStats?.cost_breakdown ? (
                  <Empty description="暂无成本数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={costOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={12}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>
                  AI 场景使用分布
                  <Tooltip title="近 30 天各 AI 工作流执行次数">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 6 }}>说明</Text>
                  </Tooltip>
                </div>
                {!aiStats || !aiStats.by_scene?.length ? (
                  <Empty description="暂无 AI 使用数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={aiSceneOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col xs={24} xl={8}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>选品阶段漏斗</div>
                {!productStats || !productStats.by_stage?.length ? (
                  <Empty description="暂无选品数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={stageOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={8}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>采购履约状态</div>
                {!overview?.purchase_total ? (
                  <Empty description="暂无采购数据" style={{ padding: '40px 0' }} />
                ) : (
                  <>
                    <ReactECharts option={purchaseStatusOption} style={{ height: 250 }} />
                    <div style={{ textAlign: 'center', marginTop: 4, fontSize: 12, color: 'rgba(0,0,0,0.45)' }}>
                      采购单总数 {overview?.purchase_total ?? 0} · 待处理 {overview?.pending_purchase_orders ?? 0}
                    </div>
                  </>
                )}
              </Card>
            </Col>
            <Col xs={24} xl={8}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12 }}>月度收入与净利润</div>
                {!profitStats?.by_month?.length ? (
                  <Empty description="暂无月度数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={warehouseOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col xs={24}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: chartCardBody }}>
                <div className="cbp-section-title" style={{ marginBottom: 12, flexWrap: 'wrap' as const }}>
                  AI 任务执行趋势（近 30 天）
                  <Space size={8} style={{ marginLeft: 12 }}>
                    <Tag color="blue" style={{ borderRadius: 4 }}>今日 {overview?.ai_runs_today ?? 0}</Tag>
                    <Tag color="purple" style={{ borderRadius: 4 }}>累计 {overview?.ai_runs_total ?? 0}</Tag>
                    <Tag color="green" style={{ borderRadius: 4 }}>
                      成功率 {formatPercent(Number(overview?.ai_success_rate ?? 0) / 100, 1)}
                    </Tag>
                  </Space>
                </div>
                {!aiStats || !aiStats.by_day?.length ? (
                  <Empty description="暂无 AI 执行数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts
                    option={{
                      tooltip: {
                        trigger: 'axis',
                        backgroundColor: 'rgba(255,255,255,0.98)',
                        borderColor: '#eef0f4',
                        borderWidth: 1,
                        textStyle: { color: 'rgba(0,0,0,0.85)', fontSize: 12 },
                        extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.10); border-radius: 10px;',
                        formatter: (params: unknown) => {
                          const arr = params as Array<{ name: string; value: number; marker: string }>;
                          if (!arr?.length) return '';
                          return `${arr[0].name}<br/>${arr[0].marker} 执行次数：${arr[0].value}`;
                        },
                      },
                      grid: { left: 48, right: 24, top: 36, bottom: 36 },
                      xAxis: {
                        type: 'category',
                        data: aiStats.by_day.map((d) => formatChartDate(d.date)),
                        axisLine: { lineStyle: { color: '#eef0f4' } },
                        axisTick: { show: false },
                        axisLabel: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
                      },
                      yAxis: {
                        type: 'value',
                        minInterval: 1,
                        splitLine: { lineStyle: { color: '#f3f4f7', type: 'dashed', opacity: 0.7 } },
                        axisLabel: { color: 'rgba(0,0,0,0.42)', fontSize: 11 },
                      },
                      series: [
                        {
                          name: '执行次数',
                          type: 'bar',
                          barWidth: 14,
                          itemStyle: {
                            borderRadius: [6, 6, 0, 0],
                            color: {
                              type: 'linear',
                              x: 0, y: 0, x2: 0, y2: 1,
                              colorStops: [
                                { offset: 0, color: '#7c3aed' },
                                { offset: 0.5, color: '#a78bfa' },
                                { offset: 1, color: '#ddd6fe' },
                              ],
                            },
                          },
                          markLine: {
                            silent: true,
                            symbol: 'none',
                            lineStyle: { color: 'rgba(114,46,209,0.45)', type: 'dashed', width: 1 },
                            label: {
                              formatter: '平均 {c}',
                              color: 'rgba(0,0,0,0.55)',
                              fontSize: 11,
                              position: 'insideEndTop',
                            },
                            data: [{ type: 'average', name: '平均' }],
                          },
                          data: aiStats.by_day.map((d) => d.count),
                        },
                      ],
                    }}
                    style={{ height: 220 }}
                  />
                )}
              </Card>
            </Col>
          </Row>

          {/* AI 数据分析助手快捷入口 */}
          <Row gutter={[14, 14]} style={{ marginTop: 14 }}>
            <Col span={24}>
              <Card
                className="cbp-enterprise-card"
                bordered={false}
                title={
                  <Space>
                    <RobotOutlined style={{ color: '#722ed1' }} />
                    <span>AI 数据分析助手</span>
                    <Tag color="purple">自然语言查询</Tag>
                  </Space>
                }
                extra={
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    输入业务问题,AI 自动生成 SQL 并返回分析结果
                  </Text>
                }
              >
                {/* 输入区 */}
                <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
                  <Input
                    placeholder="如:近30天哪些SKU利润最高? / 当前哪些库存低于安全线? / 哪些供应商履约率最高?"
                    value={aiQuestion}
                    onChange={(e) => setAiQuestion(e.target.value)}
                    onPressEnter={handleAiDataAnalysis}
                    size="large"
                  />
                  <Button
                    type="primary"
                    size="large"
                    icon={<RobotOutlined />}
                    loading={aiDataAnalyzing}
                    onClick={handleAiDataAnalysis}
                    style={{ background: '#722ed1', borderColor: '#722ed1' }}
                  >
                    分析
                  </Button>
                </Space.Compact>

                {/* 快捷问题 */}
                <Space wrap style={{ marginBottom: 16 }}>
                  {['近30天利润Top10 SKU', '低库存预警清单', 'A级供应商排行', '近30天采购单状态'].map((q) => (
                    <Tag
                      key={q}
                      style={{ cursor: 'pointer', borderRadius: 6 }}
                      color="purple"
                      onClick={() => { setAiQuestion(q); }}
                    >
                      {q}
                    </Tag>
                  ))}
                </Space>

                {/* 分析结果 */}
                {aiDataResult && (
                  <div>
                    <Alert type="success" showIcon message="业务洞察" description={aiDataResult.insight} style={{ marginBottom: 16, borderRadius: 10 }} />
                    {aiDataResult.sql && (
                      <Card size="small" type="inner" title="生成 SQL" style={{ marginBottom: 16 }}>
                        <pre className="cbp-code-block">{aiDataResult.sql}</pre>
                      </Card>
                    )}
                    {Array.isArray(aiDataResult.result) && aiDataResult.result.length > 0 && (
                      <Card size="small" type="inner" title={`查询结果 (${aiDataResult.result.length} 行)`}>
                        <Table
                          size="small"
                          dataSource={aiDataResult.result}
                          columns={Object.keys(aiDataResult.result[0]).map((key) => ({
                            title: key,
                            dataIndex: key,
                            key,
                            render: (val: any) => typeof val === 'number' ? val.toLocaleString() : String(val),
                            sorter: (a: any, b: any) => {
                              const va = a[key]; const vb = b[key];
                              if (typeof va === 'number' && typeof vb === 'number') return va - vb;
                              return String(va).localeCompare(String(vb));
                            },
                          }))}
                          pagination={{ pageSize: 8 }}
                          scroll={{ x: 'max-content' }}
                          rowKey={(_, idx) => String(idx)}
                        />
                      </Card>
                    )}
                  </div>
                )}
              </Card>
            </Col>
          </Row>
        </div>
      </Spin>
    </PageContainer>
  );
};

export default Dashboard;
