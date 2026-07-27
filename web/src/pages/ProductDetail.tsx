/**
 * 选品详情页:企业级英雄区 + 90 天趋势 + 竞品雷达/表格 + AI 洞察
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Row,
  Col,
  Tag,
  Space,
  Typography,
  Skeleton,
  Empty,
  Table,
  Button,
  Tooltip,
  Progress,
  Divider,
  message,
} from 'antd';
import {
  ArrowLeftOutlined,
  RobotOutlined,
  StarFilled,
  RiseOutlined,
  FallOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  AimOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { getProduct, listProductTrends, listProductCompetitors } from '@/api/products';
import { listAIWorkflows, runAIWorkflow } from '@/api/ai';
import {
  PRODUCT_CATEGORY_MAP,
  PRODUCT_STAGE_MAP,
  PLATFORM_MAP,
  TARGET_MARKET_MAP,
} from '@/utils/constants';
import { formatUSD, formatPercent, formatNumber, formatRating, formatDate, formatChartDate } from '@/utils/format';
import type { Product, ProductTrend, ProductCompetitor } from '@/types/api';

const { Text, Paragraph, Title } = Typography;

const ProductDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [product, setProduct] = useState<Product | null>(null);
  const [trends, setTrends] = useState<ProductTrend[]>([]);
  const [competitors, setCompetitors] = useState<ProductCompetitor[]>([]);

  // === AI 工作流深度分析结果(后端返回,优先于本地 decisionAdvice) ===
  const [aiAnalysisResult, setAiAnalysisResult] = useState<{
    score: number;
    recommendation: string;
    reasons: string[];
    risks: string[];
    suggestion: string;
    metrics?: Record<string, number>;
  } | null>(null);
  const [aiAnalyzing, setAiAnalyzing] = useState(false);
  const [aiWfId, setAiWfId] = useState<number | null>(null);

  useEffect(() => {
    const pid = Number(id);
    if (!pid) return;
    setLoading(true);
    Promise.all([getProduct(pid), listProductTrends(pid), listProductCompetitors(pid)])
      .then(([p, t, c]) => {
        setProduct(p);
        setTrends(Array.isArray(t) ? t : []);
        setCompetitors(Array.isArray(c) ? c : []);
      })
      .finally(() => setLoading(false));
  }, [id]);

  const sortedTrends = useMemo(() => {
    return [...trends].sort((a, b) => (a.stat_date < b.stat_date ? -1 : 1));
  }, [trends]);

  const trendSummary = useMemo(() => {
    if (sortedTrends.length < 2) {
      return { salesDelta: 0, searchDelta: 0, priceDelta: 0 };
    }
    const first = sortedTrends[0];
    const last = sortedTrends[sortedTrends.length - 1];
    const pct = (a: number, b: number) => (a === 0 ? 0 : ((b - a) / a) * 100);
    return {
      salesDelta: pct(Number(first.sales_volume || 0), Number(last.sales_volume || 0)),
      searchDelta: pct(Number(first.search_volume || 0), Number(last.search_volume || 0)),
      priceDelta: pct(Number(first.avg_price || 0), Number(last.avg_price || 0)),
    };
  }, [sortedTrends]);

  const trendOption = useMemo(() => {
    const dates = sortedTrends.map((t) => formatDate(t.stat_date));
    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(255,255,255,0.98)',
        borderColor: '#eef0f4',
        borderWidth: 1,
        extraCssText: 'box-shadow: 0 6px 16px rgba(0,0,0,0.08); border-radius: 8px;',
      },
      legend: { data: ['搜索量', '销量', '竞品数'], top: 0, icon: 'roundRect', itemWidth: 12, itemHeight: 8 },
      grid: { left: 50, right: 50, top: 44, bottom: 30 },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
      },
      yAxis: [
        {
          type: 'value',
          name: '数量',
          axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        },
      ],
      series: [
        {
          name: '搜索量',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: sortedTrends.map((t) => t.search_volume),
          itemStyle: { color: '#1677ff' },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(22,119,255,0.18)' },
                { offset: 1, color: 'rgba(22,119,255,0.02)' },
              ],
            },
          },
        },
        {
          name: '销量',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: sortedTrends.map((t) => t.sales_volume),
          itemStyle: { color: '#52c41a' },
        },
        {
          name: '竞品数',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: sortedTrends.map((t) => t.competitor_count),
          itemStyle: { color: '#fa8c16' },
          lineStyle: { type: 'dashed' },
        },
      ],
    };
  }, [sortedTrends]);

  const priceOption = useMemo(() => {
    const dates = sortedTrends.map((t) => formatDate(t.stat_date));
    return {
      tooltip: {
        trigger: 'axis',
        formatter: (p: any) => `${p[0]?.name || ''}<br/>均价: $${Number(p[0]?.value || 0).toFixed(2)}`,
      },
      grid: { left: 50, right: 24, top: 24, bottom: 30 },
      xAxis: {
        type: 'category',
        data: dates,
        axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
      },
      yAxis: {
        type: 'value',
        name: 'USD',
        axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
        splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
      },
      series: [
        {
          name: '市场均价',
          type: 'bar',
          barWidth: 12,
          data: sortedTrends.map((t) => Number(t.avg_price)),
          itemStyle: {
            borderRadius: [4, 4, 0, 0],
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: '#9254de' },
                { offset: 1, color: '#722ED1' },
              ],
            },
          },
        },
      ],
    };
  }, [sortedTrends]);

  // === AI 决策建议(多维度综合评分) ===
  const decisionAdvice = useMemo(() => {
    if (!product) return null;
    const aiScore = Number(product.ai_score || 0);
    const marginRate = Number(product.est_margin_rate ?? 0);
    const monthlySales = Number(product.monthly_sales || 0);
    const rating = Number(product.rating || 0);
    const reviewCount = Number(product.review_count || 0);
    const searchDelta = trendSummary.searchDelta;
    const salesDelta = trendSummary.salesDelta;

    // 各维度评分(0-100)
    const aiS = Math.min(100, aiScore);
    const marginS = Math.min(100, (marginRate / 60) * 100); // 60% 视为满分
    const salesS = Math.min(100, (monthlySales / 1000) * 100); // 1000 件/月视为满分
    const ratingS = (rating / 5) * 100;
    const reviewS = Math.min(100, (reviewCount / 500) * 100); // 500 评论视为满分
    const trendS = Math.max(0, Math.min(100, 50 + (salesDelta + searchDelta) / 4));

    const total =
      aiS * 0.30 + marginS * 0.25 + salesS * 0.15 + ratingS * 0.10 + reviewS * 0.10 + trendS * 0.10;

    let level: 'recommend' | 'caution' | 'reject';
    let levelText: string;
    let levelColor: string;
    let levelBg: string;
    let levelDesc: string;
    if (total >= 75) {
      level = 'recommend';
      levelText = '推荐进入';
      levelColor = '#389e0d';
      levelBg = 'linear-gradient(135deg, #f6ffed 0%, #f0ffeb 100%)';
      levelDesc = '综合评分优良,各项指标达推荐标准,建议尽快进入选品测试阶段';
    } else if (total >= 55) {
      level = 'caution';
      levelText = '谨慎进入';
      levelColor = '#d46b08';
      levelBg = 'linear-gradient(135deg, #fffbe6 0%, #fff7e6 100%)';
      levelDesc = '综合评分中等,部分指标存在风险,建议小批量试销验证市场反应';
    } else {
      level = 'reject';
      levelText = '不建议';
      levelColor = '#cf1322';
      levelBg = 'linear-gradient(135deg, #fff1f0 0%, #ffece6 100%)';
      levelDesc = '综合评分偏低,多项指标不达标准,建议暂缓或调整策略后再评估';
    }

    return {
      total: Math.round(total * 10) / 10,
      level,
      levelText,
      levelColor,
      levelBg,
      levelDesc,
      dimensions: [
        { name: 'AI 评分', score: Math.round(aiS), weight: 30 },
        { name: '毛利率', score: Math.round(marginS), weight: 25 },
        { name: '月销势能', score: Math.round(salesS), weight: 15 },
        { name: '商品评分', score: Math.round(ratingS), weight: 10 },
        { name: '评论基础', score: Math.round(reviewS), weight: 10 },
        { name: '趋势动能', score: Math.round(trendS), weight: 10 },
      ],
    };
  }, [product, trendSummary]);

  // === AI 工作流结果视图(派生 levelColor/levelBg,与本地 decisionAdvice 同色阶) ===
  const aiAdviceView = useMemo(() => {
    if (!aiAnalysisResult) return null;
    const score = aiAnalysisResult.score;
    let levelColor: string;
    let levelBg: string;
    if (score >= 75) {
      levelColor = '#389e0d';
      levelBg = 'linear-gradient(135deg, #f6ffed 0%, #f0ffeb 100%)';
    } else if (score >= 55) {
      levelColor = '#d46b08';
      levelBg = 'linear-gradient(135deg, #fffbe6 0%, #fff7e6 100%)';
    } else {
      levelColor = '#cf1322';
      levelBg = 'linear-gradient(135deg, #fff1f0 0%, #ffece6 100%)';
    }
    return { levelColor, levelBg };
  }, [aiAnalysisResult]);

  // === 触发 AI 工作流深度分析 ===
  const handleAiAnalyze = async () => {
    if (!product) return;
    setAiAnalyzing(true);
    try {
      // 1. 获取工作流 ID(缓存)
      let wfId = aiWfId;
      if (!wfId) {
        const wfs = await listAIWorkflows();
        const target = wfs.list.find((w) => w.code === 'wf_product_analysis');
        if (!target) {
          message.error('未找到选品分析工作流');
          return;
        }
        wfId = target.id;
        setAiWfId(wfId);
      }
      // 2. 调用工作流
      const result = await runAIWorkflow(wfId, {
        input: {
          sku: product.sku,
          name: product.name,
          category: product.category || 'beauty_device',
          list_price: String(product.list_price ?? ''),
          est_cost_price: String(product.est_cost_price ?? ''),
          monthly_sales: String(product.monthly_sales ?? ''),
        },
      });
      // 3. 解析 output
      const parsed = JSON.parse(result.output);
      // output 可能是嵌套结构:外层有 parsed 字段,或直接是结果对象
      const inner = parsed.parsed || parsed;
      setAiAnalysisResult({
        score: Number(inner.score || 0),
        recommendation: inner.recommendation || '',
        reasons: Array.isArray(inner.reasons) ? inner.reasons : [],
        risks: Array.isArray(inner.risks) ? inner.risks : [],
        suggestion: inner.suggestion || '',
        metrics: inner.metrics,
      });
      message.success('AI 分析完成');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'AI 分析失败');
    } finally {
      setAiAnalyzing(false);
    }
  };

  // === 风险提示 ===
  const riskItems = useMemo(() => {
    if (!product) return [];
    const items: { level: 'high' | 'medium' | 'low'; text: string }[] = [];
    const aiScore = Number(product.ai_score || 0);
    const marginRate = Number(product.est_margin_rate ?? 0);
    const monthlySales = Number(product.monthly_sales || 0);
    const rating = Number(product.rating || 0);
    const reviewCount = Number(product.review_count || 0);
    const searchDelta = trendSummary.searchDelta;
    const salesDelta = trendSummary.salesDelta;
    const lastCompetitorCount = sortedTrends.length
      ? Number(sortedTrends[sortedTrends.length - 1].competitor_count || 0)
      : 0;

    if (aiScore < 65)
      items.push({ level: 'high', text: `AI 评分偏低(${aiScore.toFixed(0)}),建议优化选品策略` });
    if (marginRate < 25 && marginRate > 0)
      items.push({ level: 'high', text: `预估毛利率仅 ${marginRate.toFixed(1)}%,低于 25% 安全线` });
    if (rating > 0 && rating < 4.0)
      items.push({ level: 'medium', text: `商品评分 ${rating.toFixed(2)},低于 4.0 行业基准` });
    if (reviewCount > 0 && reviewCount < 50)
      items.push({ level: 'medium', text: `评论数仅 ${reviewCount} 条,口碑基础薄弱` });
    if (monthlySales > 0 && monthlySales < 100)
      items.push({ level: 'medium', text: `预估月销 ${monthlySales} 件,市场体量偏小` });
    if (lastCompetitorCount > 80)
      items.push({ level: 'high', text: `市场竞品 ${lastCompetitorCount} 个,红海竞争激烈` });
    if (searchDelta < -10)
      items.push({ level: 'medium', text: `近 90 天搜索热度下滑 ${Math.abs(searchDelta).toFixed(1)}%` });
    if (salesDelta < -10)
      items.push({ level: 'high', text: `近 90 天销量下滑 ${Math.abs(salesDelta).toFixed(1)}%,需求疲软` });

    if (items.length === 0) {
      items.push({ level: 'low', text: '暂未识别明显风险,各项指标处于合理区间' });
    }
    return items;
  }, [product, sortedTrends, trendSummary]);

  // === 预估成本结构(基于售价与采购成本反推) ===
  const costStructureOption: EChartsOption | null = useMemo(() => {
    if (!product) return null;
    const listPrice = Number(product.list_price || 0);
    const costPrice = Number(product.est_cost_price || 0);
    if (listPrice <= 0 || costPrice <= 0) return null;

    const goodsCost = costPrice;
    const freightCost = costPrice * 0.18;
    const platformFee = listPrice * 0.15;
    const adCost = listPrice * 0.10;
    const taxCost = listPrice * 0.08;
    const refundCost = listPrice * 0.03;
    const otherCost = listPrice * 0.04;
    const totalCost = goodsCost + freightCost + platformFee + adCost + taxCost + refundCost + otherCost;
    const netProfit = listPrice - totalCost;

    const palette: Record<string, string> = {
      货物成本: '#1677ff',
      头程运费: '#52c41a',
      平台佣金: '#fa8c16',
      广告费: '#722ed1',
      税费: '#13c2c2',
      退款损失: '#eb2f96',
      其他: '#8c8c8c',
      净利润: '#52c41a',
    };

    const dataItems = [
      { name: '货物成本', value: Number(goodsCost.toFixed(2)) },
      { name: '头程运费', value: Number(freightCost.toFixed(2)) },
      { name: '平台佣金', value: Number(platformFee.toFixed(2)) },
      { name: '广告费', value: Number(adCost.toFixed(2)) },
      { name: '税费', value: Number(taxCost.toFixed(2)) },
      { name: '退款损失', value: Number(refundCost.toFixed(2)) },
      { name: '其他', value: Number(otherCost.toFixed(2)) },
    ];
    if (netProfit > 0) {
      dataItems.push({ name: '净利润', value: Number(netProfit.toFixed(2)) });
    }

    return {
      tooltip: {
        trigger: 'item',
        formatter: (params: unknown) => {
          const p = params as { name: string; value: number; percent: number };
          return `${p.name}<br/>金额: $${p.value.toFixed(2)}<br/>占比: ${p.percent}%`;
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
        itemGap: 10,
        textStyle: { color: 'rgba(0,0,0,0.65)', fontSize: 12 },
      },
      series: [
        {
          name: '成本结构',
          type: 'pie',
          radius: ['52%', '72%'],
          center: ['62%', '50%'],
          avoidLabelOverlap: true,
          itemStyle: { borderRadius: 4, borderColor: '#fff', borderWidth: 2 },
          label: {
            show: true,
            position: 'center',
            formatter: () => {
              if (netProfit <= 0) {
                return `{a|售价}\n{b|$${listPrice.toFixed(2)}}\n{c|亏损}`;
              }
              const margin = ((netProfit / listPrice) * 100).toFixed(1);
              return `{a|净利率}\n{b|${margin}%}\n{c|净利 $${netProfit.toFixed(2)}}`;
            },
            rich: {
              a: { fontSize: 11, color: 'rgba(0,0,0,0.45)', padding: [2, 0, 2, 0] },
              b: {
                fontSize: 20,
                fontWeight: 600,
                color: netProfit > 0 ? '#389e0d' : '#cf1322',
                padding: [4, 0, 4, 0],
              },
              c: {
                fontSize: 11,
                color: netProfit > 0 ? 'rgba(0,0,0,0.55)' : '#cf1322',
              },
            },
          },
          emphasis: {
            label: { show: true, fontSize: 13, fontWeight: 'bold' },
            scaleSize: 6,
            itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.10)' },
          },
          data: dataItems.map((d) => ({
            name: d.name,
            value: d.value,
            itemStyle: { color: palette[d.name] },
          })),
        },
      ],
    };
  }, [product]);

  const marketTrendOption: EChartsOption = useMemo(() => {
    const dates = sortedTrends.map((t) => formatChartDate(t.stat_date));
    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(255,255,255,0.98)',
        borderColor: '#eef0f4',
        borderWidth: 1,
        extraCssText: 'box-shadow: 0 6px 16px rgba(0,0,0,0.08); border-radius: 8px;',
      },
      legend: {
        data: ['搜索量', '销量', '市场均价'],
        top: 4,
        right: 8,
        icon: 'roundRect',
        itemWidth: 12,
        itemHeight: 8,
        textStyle: { fontSize: 12, color: 'rgba(0,0,0,0.65)' },
      },
      grid: { left: 56, right: 60, top: 44, bottom: 32 },
      xAxis: {
        type: 'category',
        data: dates,
        boundaryGap: false,
        axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
        axisLine: { lineStyle: { color: '#eef0f4' } },
        axisTick: { show: false },
      },
      yAxis: [
        {
          type: 'value',
          name: '搜索量/销量',
          nameTextStyle: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          axisLabel: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          splitLine: { lineStyle: { color: '#f0f2f5', type: 'dashed' } },
        },
        {
          type: 'value',
          name: '均价(USD)',
          nameTextStyle: { fontSize: 11, color: 'rgba(0,0,0,0.45)' },
          axisLabel: {
            fontSize: 11,
            color: 'rgba(0,0,0,0.45)',
            formatter: '${value}',
          },
          splitLine: { show: false },
        },
      ],
      series: [
        {
          name: '搜索量',
          type: 'line',
          smooth: true,
          showSymbol: false,
          yAxisIndex: 0,
          data: sortedTrends.map((t) => Number(t.search_volume || 0)),
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
          name: '销量',
          type: 'line',
          smooth: true,
          showSymbol: false,
          yAxisIndex: 0,
          data: sortedTrends.map((t) => Number(t.sales_volume || 0)),
          itemStyle: { color: '#52c41a' },
          lineStyle: { width: 2.5 },
        },
        {
          name: '市场均价',
          type: 'line',
          smooth: true,
          showSymbol: false,
          yAxisIndex: 1,
          data: sortedTrends.map((t) => Number(t.avg_price || 0)),
          itemStyle: { color: '#fa8c16' },
          lineStyle: { width: 2, type: 'dashed' },
        },
      ],
    };
  }, [sortedTrends]);

  const competitorRadarOption: EChartsOption | null = useMemo(() => {
    if (!competitors.length) return null;
    const list = competitors.slice(0, 5);
    const prices = list.map((c) => Number(c.price || 0));
    const reviews = list.map((c) => Number(c.review_count || 0));
    const sales = list.map((c) => Number(c.sales_est || 0));
    const maxPrice = Math.max(...prices, 1);
    const maxReviews = Math.max(...reviews, 1);
    const maxSales = Math.max(...sales, 1);
    const cpRaw = list.map((c) => {
      const p = Number(c.price || 0);
      const r = Number(c.rating || 0);
      const s = Number(c.sales_est || 0);
      return p > 0 ? (r / 5) * (s / maxSales) / p : 0;
    });
    const maxCp = Math.max(...cpRaw, 0.0001);
    const palette = [
      { line: '#1677ff', area: 'rgba(22,119,255,0.14)' },
      { line: '#722ed1', area: 'rgba(114,46,209,0.14)' },
      { line: '#fa8c16', area: 'rgba(250,140,22,0.14)' },
      { line: '#13c2c2', area: 'rgba(19,194,194,0.14)' },
      { line: '#eb2f96', area: 'rgba(235,47,150,0.14)' },
    ];
    return {
      tooltip: { trigger: 'item' },
      legend: {
        data: list.map((c) => c.brand || c.competitor_asin),
        bottom: 0,
        type: 'scroll',
        textStyle: { fontSize: 11, color: 'rgba(0,0,0,0.65)' },
      },
      radar: {
        indicator: [
          { name: '价格竞争力', max: 100 },
          { name: '评分', max: 100 },
          { name: '评论数', max: 100 },
          { name: '预估销量', max: 100 },
          { name: '性价比', max: 100 },
        ],
        radius: '60%',
        center: ['50%', '48%'],
        splitNumber: 4,
        axisName: { color: 'rgba(0,0,0,0.55)', fontSize: 11 },
        splitLine: { lineStyle: { color: '#eef0f4', type: 'dashed' } },
        splitArea: {
          areaStyle: {
            color: ['rgba(22,119,255,0.02)', 'rgba(22,119,255,0.05)'],
          },
        },
        axisLine: { lineStyle: { color: '#eef0f4' } },
      },
      series: [
        {
          type: 'radar',
          data: list.map((c, idx) => {
            const price = Number(c.price || 0);
            const rating = Number(c.rating || 0);
            const review = Number(c.review_count || 0);
            const sale = Number(c.sales_est || 0);
            return {
              name: c.brand || c.competitor_asin,
              value: [
                Math.round((1 - price / maxPrice) * 100),
                Math.round((rating / 5) * 100),
                Math.round((review / maxReviews) * 100),
                Math.round((sale / maxSales) * 100),
                Math.round((cpRaw[idx] / maxCp) * 100),
              ],
              lineStyle: { width: 1.8, color: palette[idx % palette.length].line },
              itemStyle: { color: palette[idx % palette.length].line },
              areaStyle: { color: palette[idx % palette.length].area },
            };
          }),
        },
      ],
    };
  }, [competitors]);

  const competitorColumns = [
    {
      title: '竞品 ASIN',
      dataIndex: 'competitor_asin',
      width: 140,
      render: (v: string, r: ProductCompetitor) =>
        r.listing_url ? (
          <a href={r.listing_url} target="_blank" rel="noreferrer">
            {v}
          </a>
        ) : (
          <Text strong>{v}</Text>
        ),
    },
    { title: '品牌', dataIndex: 'brand', width: 120, ellipsis: true },
    {
      title: '售价',
      dataIndex: 'price',
      width: 90,
      align: 'right' as const,
      render: (v: number) => formatUSD(v),
      sorter: (a: ProductCompetitor, b: ProductCompetitor) => Number(a.price) - Number(b.price),
    },
    {
      title: '预估月销',
      dataIndex: 'sales_est',
      width: 100,
      align: 'right' as const,
      render: (v: number) => formatNumber(v),
      sorter: (a: ProductCompetitor, b: ProductCompetitor) => a.sales_est - b.sales_est,
    },
    {
      title: '评论数',
      dataIndex: 'review_count',
      width: 90,
      align: 'right' as const,
      render: (v: number) => formatNumber(v),
      sorter: (a: ProductCompetitor, b: ProductCompetitor) => a.review_count - b.review_count,
    },
    {
      title: '评分',
      dataIndex: 'rating',
      width: 90,
      align: 'right' as const,
      render: (v: number) => (
        <Space size={4}>
          <StarFilled style={{ color: '#faad14', fontSize: 12 }} />
          {formatRating(v)}
        </Space>
      ),
    },
    {
      title: '相对本 SKU',
      width: 120,
      align: 'right' as const,
      render: (_: unknown, r: ProductCompetitor) => {
        if (!product) return '-';
        const delta = Number(r.price || 0) - Number(product.list_price || 0);
        if (Math.abs(delta) < 0.01) return <Text type="secondary">持平</Text>;
        return (
          <Text type={delta > 0 ? 'success' : 'danger'}>
            {delta > 0 ? '+' : ''}
            {formatUSD(delta)}
          </Text>
        );
      },
    },
  ];

  const DeltaTag: React.FC<{ value: number; label: string }> = ({ value, label }) => {
    const up = value >= 0;
    return (
      <Tag color={up ? 'success' : 'error'} style={{ borderRadius: 6 }}>
        {up ? <RiseOutlined /> : <FallOutlined />} {label} {Math.abs(value).toFixed(1)}%
      </Tag>
    );
  };

  return (
    <PageContainer
      title="选品详情"
      breadcrumb={{
        items: [
          { title: '选品管理', onClick: () => navigate('/products') },
          { title: `SKU: ${product?.sku || id}` },
        ],
      }}
      header={{
        title: '选品详情',
        onBack: () => navigate('/products'),
      }}
    >
      <div style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/products')}>
          返回选品列表
        </Button>
      </div>

      {loading ? (
        <Skeleton active paragraph={{ rows: 10 }} />
      ) : !product ? (
        <Empty description="未找到该选品" />
      ) : (
        <>
          <Card className="cbp-detail-hero" bordered={false} styles={{ body: { padding: 20 } }} style={{ marginBottom: 16 }}>
            <Row gutter={[24, 16]}>
              <Col xs={24} xl={14}>
                <Space align="start" size={14} style={{ width: '100%' }}>
                  <div className="cbp-icon-tile">
                    <ThunderboltOutlined />
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap', marginBottom: 10 }}>
                      <Title level={4} style={{ margin: 0 }}>
                        {product.name}
                      </Title>
                      <Tag color="blue" style={{ borderRadius: 6 }}>
                        {product.sku}
                      </Tag>
                      {product.asin && <Tag style={{ borderRadius: 6 }}>ASIN: {product.asin}</Tag>}
                    </div>
                    <Space size={[8, 8]} wrap>
                      <StatusTag status={product.category} map={PRODUCT_CATEGORY_MAP} />
                      <StatusTag status={product.stage} map={PRODUCT_STAGE_MAP} />
                      <StatusTag status={product.platform} map={PLATFORM_MAP} />
                      <Tag>目标市场: {TARGET_MARKET_MAP[product.target_market] || product.target_market}</Tag>
                    </Space>
                    {product.ai_insight && (
                      <div className="cbp-ai-insight" style={{ marginTop: 16 }}>
                        <Space align="start">
                          <RobotOutlined style={{ color: '#2F54EB', marginTop: 4 }} />
                          <Paragraph style={{ margin: 0 }}>{product.ai_insight}</Paragraph>
                        </Space>
                      </div>
                    )}
                  </div>
                </Space>
              </Col>
              <Col xs={24} xl={10}>
                <Row gutter={[12, 12]}>
                  <Col span={12}>
                    <div className="cbp-metric-tile">
                      <div className="label">上架价</div>
                      <div className="value" style={{ color: '#1677ff' }}>
                        {formatUSD(product.list_price)}
                      </div>
                      <div className="hint">目标售价</div>
                    </div>
                  </Col>
                  <Col span={12}>
                    <div className="cbp-metric-tile">
                      <div className="label">AI 评分</div>
                      <div className="value" style={{ color: '#722ed1' }}>
                        <StarFilled style={{ color: '#faad14', fontSize: 14, marginRight: 6 }} />
                        {product.ai_score}
                      </div>
                      <Progress
                        percent={Number(product.ai_score || 0)}
                        showInfo={false}
                        size="small"
                        strokeColor={Number(product.ai_score || 0) >= 80 ? '#52c41a' : '#1677ff'}
                        style={{ marginTop: 8 }}
                      />
                    </div>
                  </Col>
                  <Col span={12}>
                    <div className="cbp-metric-tile">
                      <div className="label">预估毛利率</div>
                      <div
                        className="value"
                        style={{
                          color: Number(product.est_margin_rate ?? 0) >= 60 ? '#52c41a' : 'inherit',
                        }}
                      >
                        {formatPercent(Number(product.est_margin_rate ?? 0) / 100, 1)}
                      </div>
                      <div className="hint">含基础费用测算</div>
                    </div>
                  </Col>
                  <Col span={12}>
                    <div className="cbp-metric-tile">
                      <div className="label">月销量 / 评分</div>
                      <div className="value">{formatNumber(product.monthly_sales)}</div>
                      <div className="hint">
                        <StarFilled style={{ color: '#faad14', fontSize: 11, marginRight: 4 }} />
                        {formatRating(product.rating)} · 评论 {formatNumber(product.review_count)}
                      </div>
                    </div>
                  </Col>
                </Row>
              </Col>
            </Row>
          </Card>

          {/* AI 决策建议 + 风险提示 */}
          {decisionAdvice && (
            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              <Col xs={24} xl={16}>
                <Card
                  bordered={false}
                  styles={{ body: { padding: 0 } }}
                  style={{ height: '100%', overflow: 'hidden', borderRadius: 12 }}
                  className="cbp-chart-card"
                >
                  <div
                    className="cbp-decision-card"
                    style={{ background: aiAdviceView ? aiAdviceView.levelBg : decisionAdvice.levelBg }}
                  >
                    <div className="cbp-decision-head">
                      <div
                        className="cbp-decision-icon"
                        style={{ color: aiAdviceView ? aiAdviceView.levelColor : decisionAdvice.levelColor }}
                      >
                        <AimOutlined />
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div className="cbp-decision-label">
                          AI 选品决策建议
                          {aiAnalysisResult ? (
                            <Tag color="blue" style={{ marginLeft: 8, borderRadius: 6, fontSize: 11 }}>
                              AI 工作流
                            </Tag>
                          ) : (
                            <Tag color="blue" style={{ marginLeft: 8, borderRadius: 6, fontSize: 11 }}>
                              综合评分
                            </Tag>
                          )}
                        </div>
                        <div className="cbp-decision-desc">
                          {aiAnalysisResult ? aiAnalysisResult.suggestion || '基于 AI 工作流的深度分析结果' : decisionAdvice.levelDesc}
                        </div>
                      </div>
                      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 8 }}>
                        <Button
                          type="primary"
                          size="small"
                          icon={<RobotOutlined />}
                          loading={aiAnalyzing}
                          onClick={handleAiAnalyze}
                        >
                          {aiAnalysisResult ? '重新分析' : 'AI 深度分析'}
                        </Button>
                        <div
                          className="cbp-decision-result"
                          style={{
                            color: aiAdviceView ? aiAdviceView.levelColor : decisionAdvice.levelColor,
                            borderColor: aiAdviceView ? aiAdviceView.levelColor : decisionAdvice.levelColor,
                          }}
                        >
                          <span className="cbp-decision-result-num">
                            {aiAnalysisResult ? aiAnalysisResult.score : decisionAdvice.total}
                          </span>
                          <span className="cbp-decision-result-text">
                            {aiAnalysisResult ? aiAnalysisResult.recommendation || 'AI 建议' : decisionAdvice.levelText}
                          </span>
                        </div>
                      </div>
                    </div>
                    <div className="cbp-decision-dims">
                      {aiAnalysisResult && aiAnalysisResult.metrics
                        ? Object.entries(aiAnalysisResult.metrics).map(([name, score]) => {
                            const s = Math.min(100, Math.max(0, Math.round(Number(score))));
                            return (
                              <div key={name} className="cbp-decision-dim">
                                <div className="cbp-decision-dim-head">
                                  <span>{name}</span>
                                  <span className="cbp-decision-dim-score">
                                    {s}
                                    <Text type="secondary" style={{ fontSize: 10, marginLeft: 2 }}>
                                      /100
                                    </Text>
                                  </span>
                                </div>
                                <Progress
                                  percent={s}
                                  showInfo={false}
                                  size="small"
                                  strokeColor={s >= 75 ? '#52c41a' : s >= 50 ? '#faad14' : '#cf1322'}
                                />
                              </div>
                            );
                          })
                        : decisionAdvice.dimensions.map((d) => (
                            <div key={d.name} className="cbp-decision-dim">
                              <div className="cbp-decision-dim-head">
                                <span>{d.name}</span>
                                <span className="cbp-decision-dim-score">
                                  {d.score}
                                  <Text type="secondary" style={{ fontSize: 10, marginLeft: 2 }}>
                                    /100
                                  </Text>
                                </span>
                              </div>
                              <Progress
                                percent={d.score}
                                showInfo={false}
                                size="small"
                                strokeColor={
                                  d.score >= 75 ? '#52c41a' : d.score >= 50 ? '#faad14' : '#cf1322'
                                }
                              />
                              <div className="cbp-decision-dim-weight">权重 {d.weight}%</div>
                            </div>
                          ))}
                    </div>
                    {aiAnalysisResult && (
                      <div style={{ padding: '12px 18px', background: 'rgba(255,255,255,0.6)' }}>
                        {aiAnalysisResult.reasons.length > 0 && (
                          <div style={{ marginBottom: 8 }}>
                            <div style={{ fontSize: 12, color: 'rgba(0,0,0,0.45)', marginBottom: 4 }}>
                              支撑理由
                            </div>
                            <ul style={{ margin: 0, paddingLeft: 20 }}>
                              {aiAnalysisResult.reasons.map((r, i) => (
                                <li
                                  key={i}
                                  style={{ fontSize: 13, color: 'rgba(0,0,0,0.75)', marginBottom: 2 }}
                                >
                                  {r}
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                        {aiAnalysisResult.risks.length > 0 && (
                          <div style={{ marginBottom: 8 }}>
                            <div style={{ fontSize: 12, color: 'rgba(0,0,0,0.45)', marginBottom: 4 }}>
                              潜在风险
                            </div>
                            <ul style={{ margin: 0, paddingLeft: 20 }}>
                              {aiAnalysisResult.risks.map((r, i) => (
                                <li
                                  key={i}
                                  style={{ fontSize: 13, color: 'rgba(0,0,0,0.75)', marginBottom: 2 }}
                                >
                                  {r}
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                        {aiAnalysisResult.suggestion && (
                          <div>
                            <div style={{ fontSize: 12, color: 'rgba(0,0,0,0.45)', marginBottom: 4 }}>
                              行动建议
                            </div>
                            <Paragraph style={{ margin: 0, fontSize: 13, color: 'rgba(0,0,0,0.85)' }}>
                              {aiAnalysisResult.suggestion}
                            </Paragraph>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </Card>
              </Col>
              <Col xs={24} xl={8}>
                <Card
                  bordered={false}
                  styles={{ body: { padding: '14px 18px' } }}
                  style={{ height: '100%', borderRadius: 12 }}
                  className="cbp-chart-card"
                >
                  <div className="cbp-section-title" style={{ marginBottom: 12, padding: '4px 0' }}>
                    <WarningOutlined style={{ color: '#fa8c16' }} />
                    风险提示
                  </div>
                  <div className="cbp-risk-list">
                    {riskItems.map((r, idx) => (
                      <div key={idx} className={`cbp-risk-item cbp-risk-${r.level}`}>
                        <span className="cbp-risk-dot" />
                        <Text style={{ fontSize: 13, color: 'rgba(0,0,0,0.78)' }}>{r.text}</Text>
                      </div>
                    ))}
                  </div>
                </Card>
              </Col>
            </Row>
          )}

          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={24} xl={12}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
                <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
                  近30天市场趋势
                  <Tooltip title="左轴展示搜索量与销量走势，右轴展示市场均价">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>
                      ?
                    </Text>
                  </Tooltip>
                </div>
                {sortedTrends.length === 0 ? (
                  <Empty description="暂无趋势数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={marketTrendOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={12}>
              <Card className="cbp-chart-card" bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
                <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
                  竞品雷达对比
                  <Tooltip title="从价格竞争力、评分、评论数、预估销量、性价比五个维度对比竞品">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>
                      ?
                    </Text>
                  </Tooltip>
                </div>
                {competitorRadarOption === null ? (
                  <Empty description="暂无竞品数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={competitorRadarOption} style={{ height: 280 }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
            <Col xs={24} xl={16}>
              <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 8 }}>
                  <div className="cbp-section-title" style={{ padding: '8px 0' }}>
                    90 天市场趋势
                    <Tooltip title="展示搜索热度、销量走势与竞品密度变化">
                      <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>
                        ?
                      </Text>
                    </Tooltip>
                  </div>
                  <Space wrap size={6}>
                    <DeltaTag value={trendSummary.searchDelta} label="搜索" />
                    <DeltaTag value={trendSummary.salesDelta} label="销量" />
                    <DeltaTag value={trendSummary.priceDelta} label="均价" />
                  </Space>
                </div>
                {sortedTrends.length === 0 ? (
                  <Empty description="暂无趋势数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={trendOption} style={{ height: 320 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={8}>
              <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }}>
                <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
                  市场均价走势
                </div>
                {sortedTrends.length === 0 ? (
                  <Empty description="暂无均价数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={priceOption} style={{ height: 320 }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]}>
            <Col xs={24} xl={10}>
              <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }} className="cbp-chart-card">
                <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
                  预估成本结构
                  <Tooltip title="基于售价与采购成本反推,各项成本占比与净利率">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>
                      ?
                    </Text>
                  </Tooltip>
                </div>
                {costStructureOption === null ? (
                  <Empty description="暂无成本数据" style={{ padding: '40px 0' }} />
                ) : (
                  <ReactECharts option={costStructureOption} style={{ height: 320 }} />
                )}
              </Card>
            </Col>
            <Col xs={24} xl={14}>
              <Card bordered={false} styles={{ body: { padding: '12px 16px 16px' } }} style={{ height: '100%' }} className="cbp-chart-card">
                <div className="cbp-section-title" style={{ marginBottom: 8, padding: '8px 0' }}>
                  竞品监控
                  <Tooltip title="支持按售价/月销/评论数排序,可跳转竞品 Listing">
                    <Text type="secondary" style={{ fontSize: 12, fontWeight: 400, marginLeft: 4 }}>
                      ?
                    </Text>
                  </Tooltip>
                </div>
                <Divider style={{ margin: '4px 0 12px' }} />
                <Table
                  rowKey="id"
                  columns={competitorColumns}
                  dataSource={competitors}
                  pagination={{ pageSize: 6, showSizeChanger: false }}
                  scroll={{ x: 720 }}
                  size="small"
                  locale={{ emptyText: <Empty description="暂无竞品数据" /> }}
                />
              </Card>
            </Col>
          </Row>
        </>
      )}
    </PageContainer>
  );
};

export default ProductDetail;
