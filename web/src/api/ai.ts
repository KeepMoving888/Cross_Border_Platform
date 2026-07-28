/**
 * AI 工作流 API
 * - 全量对接后端:列表/详情/执行/历史
 * - 后端未配置 LLM_API_KEY 时自动走 BuiltinLLMProvider,返回基于输入的结构化结果
 * - 配置 LLM_API_KEY 后无缝切换至真实 GLM/Claude/DeepSeek/Qwen 调用
 */
import { get, post } from './client';
import type {
  AIWorkflow,
  AIWorkflowRunRequest,
  AIWorkflowRunResult,
  KnowledgeBase,
  KnowledgeDocument,
  PageResponse,
  RAGSearchResult,
} from '@/types/api';

export async function listAIWorkflows(): Promise<PageResponse<AIWorkflow>> {
  return get<PageResponse<AIWorkflow>>('/ai/workflows');
}

export async function getAIWorkflow(id: number): Promise<AIWorkflow> {
  return get<AIWorkflow>(`/ai/workflows/${id}`);
}

/**
 * 执行工作流:真实调用后端 POST /ai/workflows/:id/run
 * 后端引擎会按工作流定义(input -> llm/rag/text2sql -> output)拓扑执行
 */
export async function runAIWorkflow(
  id: number,
  payload: AIWorkflowRunRequest,
  meta?: { code?: string },
): Promise<AIWorkflowRunResult> {
  // 模拟 AI 推理过程的等待体验(后端真实执行可能需 1-5s,这里前端加最小展示时长)
  const minDelay = 600;
  const start = Date.now();
  const result = await post<AIWorkflowRunResult>(`/ai/workflows/${id}/run`, payload);
  const elapsed = Date.now() - start;
  if (elapsed < minDelay) {
    await new Promise((r) => setTimeout(r, minDelay - elapsed));
  }
  return result;
}

export async function listAIRuns(
  params: { workflow_code?: string; status?: string; page?: number; page_size?: number } = {},
): Promise<PageResponse<AIWorkflowRunResult>> {
  return get<PageResponse<AIWorkflowRunResult>>('/ai/runs', params as Record<string, unknown>);
}

// ============== 知识库管理(RAG) ==============

/** 知识库列表 */
export async function listKnowledgeBases(): Promise<KnowledgeBase[]> {
  return get<KnowledgeBase[]>('/ai/knowledge-bases');
}

/** 创建知识库 */
export async function createKnowledgeBase(payload: {
  name: string;
  code?: string;
  description?: string;
  type?: string;
}): Promise<KnowledgeBase> {
  return post<KnowledgeBase>('/ai/knowledge-bases', payload);
}

/** 知识库下的文档列表 */
export async function listDocuments(knowledgeBaseID: number): Promise<KnowledgeDocument[]> {
  return get<KnowledgeDocument[]>(`/ai/knowledge-bases/${knowledgeBaseID}/documents`);
}

/** 上传文档(纯文本 content,后端异步分块+向量化入库) */
export async function uploadDocument(
  knowledgeBaseID: number,
  payload: { title: string; content: string; source?: string },
): Promise<KnowledgeDocument> {
  return post<KnowledgeDocument>(`/ai/knowledge-bases/${knowledgeBaseID}/documents`, payload);
}

/** 上传文件文档(支持 PDF/Word/Markdown/TXT,multipart/form-data)
 * 后端解析文件为纯文本,再走分块+向量化入库
 */
export async function uploadDocumentFile(
  knowledgeBaseID: number,
  file: File,
  title?: string,
): Promise<KnowledgeDocument> {
  const formData = new FormData();
	formData.append('file', file);
	if (title) {
		formData.append('title', title);
	}
	// 直接 fetch,client.ts 的 post 不支持 FormData
	const token = localStorage.getItem('token') || '';
	const resp = await fetch(`/api/v1/ai/knowledge-bases/${knowledgeBaseID}/documents/upload`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${token}`,
		},
		body: formData,
	});
	const json = await resp.json();
	if (!resp.ok || json.code !== 0) {
		throw new Error(json.message || '文件上传失败');
	}
	return json.data as KnowledgeDocument;
}

/** RAG 检索测试:输入 query 返回 top-K 文档分块 */
export async function ragSearch(payload: {
  query: string;
  knowledge_base_id: number;
  top_k?: number;
}): Promise<RAGSearchResult> {
  return post<RAGSearchResult>('/ai/rag/search', payload);
}

// ============== RAG 监控(对接 Prometheus HTTP API) ==============

/** Prometheus 查询响应 */
interface PrometheusResponse {
  status: string;
  data: {
    resultType: string;
    result: Array<{
      metric: Record<string, string>;
      value: [number, string];
    }>;
  };
}

/** Prometheus range 查询响应(时间序列) */
interface PrometheusRangeResponse {
  status: string;
  data: {
    resultType: string;
    result: Array<{
      metric: Record<string, string>;
      values: Array<[number, string]>;
    }>;
  };
}

/** RAG 监控指标聚合 */
export interface RAGMetrics {
  searchSuccessRate: number;      // 检索成功率 [0,1]
  searchTotal: number;            // 总检索次数
  cacheHitRate: number;           // 缓存命中率 [0,1]
  avgSearchDuration: number;      // 平均检索延迟(秒)
  fallbackTotal: number;          // 降级次数
  rerankTotal: number;            // 重排序次数
  indexedDocs: number;            // 已入库文档数
  strategyDistribution: Array<{   // 检索策略分布
    strategy: string;
    count: number;
  }>;
}

/** RAG 趋势数据(时间序列) */
export interface RAGTrendData {
  timestamps: number[];           // 时间戳数组(Unix 秒)
  successRate: number[];          // 检索成功率序列 [0,1]
  avgDuration: number[];          // 平均延迟序列(秒)
  searchCount: number[];          // 检索次数序列(区间增量)
}

/** 监控时间范围 */
export type MetricsTimeRange = '5m' | '1h' | '24h';

const PROMETHEUS_URL = '/prometheus/api/v1/query';
const PROMETHEUS_RANGE_URL = '/prometheus/api/v1/query_range';

/** 查询 Prometheus 单值指标 */
async function queryPrometheus(query: string): Promise<number> {
  try {
    const resp = await fetch(`${PROMETHEUS_URL}?query=${encodeURIComponent(query)}`);
    const json: PrometheusResponse = await resp.json();
    if (json.status === 'success' && json.data.result.length > 0) {
      return parseFloat(json.data.result[0].value[1]) || 0;
    }
  } catch {
    // Prometheus 不可用时静默
  }
  return 0;
}

/** 查询 Prometheus 多值指标(按 label 分组) */
async function queryPrometheusByLabel(query: string): Promise<Array<{ label: string; value: number }>> {
  try {
    const resp = await fetch(`${PROMETHEUS_URL}?query=${encodeURIComponent(query)}`);
    const json: PrometheusResponse = await resp.json();
    if (json.status === 'success') {
      return json.data.result.map((r) => ({
        label: r.metric.strategy || r.metric.status || 'unknown',
        value: parseFloat(r.value[1]) || 0,
      }));
    }
  } catch {
    // 静默
  }
  return [];
}

/** 查询 Prometheus range 查询(时间序列) */
async function queryPrometheusRange(
  query: string,
  range: MetricsTimeRange,
): Promise<Array<[number, number]>> {
  try {
    const now = Math.floor(Date.now() / 1000);
    const rangeSeconds: Record<MetricsTimeRange, number> = { '5m': 300, '1h': 3600, '24h': 86400 };
    const step: Record<MetricsTimeRange, number> = { '5m': 15, '1h': 60, '24h': 600 };
    const start = now - rangeSeconds[range];

    const params = new URLSearchParams({
      query,
      start: String(start),
      end: String(now),
      step: String(step[range]),
    });
    const resp = await fetch(`${PROMETHEUS_RANGE_URL}?${params.toString()}`);
    const json: PrometheusRangeResponse = await resp.json();
    if (json.status === 'success' && json.data.result.length > 0) {
      return json.data.result[0].values.map(([ts, val]) => [ts, parseFloat(val) || 0]);
    }
  } catch {
    // 静默
  }
  return [];
}

/** 获取 RAG 监控指标(并行查询 Prometheus) */
export async function getRAGMetrics(): Promise<RAGMetrics> {
  const [
    successRate,
    total,
    cacheHits,
    cacheTotal,
    avgDuration,
    fallback,
    rerank,
    indexed,
    strategyDist,
  ] = await Promise.all([
    queryPrometheus('sum(rag_search_total{status="success"}) / sum(rag_search_total)'),
    queryPrometheus('sum(rag_search_total)'),
    queryPrometheus('sum(rag_cache_hits_total)'),
    queryPrometheus('sum(rag_search_total)'),
    queryPrometheus('avg(rag_search_duration_seconds_sum / rag_search_duration_seconds_count)'),
    queryPrometheus('sum(rag_fallback_total)'),
    queryPrometheus('sum(rag_rerank_total)'),
    queryPrometheus('sum(rag_index_docs_total{status="success"})'),
    queryPrometheusByLabel('sum(rag_search_total) by (strategy)'),
  ]);

  return {
    searchSuccessRate: successRate,
    searchTotal: total,
    cacheHitRate: cacheTotal > 0 ? cacheHits / cacheTotal : 0,
    avgSearchDuration: avgDuration,
    fallbackTotal: fallback,
    rerankTotal: rerank,
    indexedDocs: indexed,
    strategyDistribution: strategyDist.map((s) => ({ strategy: s.label, count: s.value })),
  };
}

/** 获取 RAG 趋势数据(时间序列,用于趋势图) */
export async function getRAGTrend(range: MetricsTimeRange): Promise<RAGTrendData> {
  const [successRateSeries, durationSeries, countSeries] = await Promise.all([
    queryPrometheusRange(
      'sum(rag_search_total{status="success"}) / sum(rag_search_total)',
      range,
    ),
    queryPrometheusRange(
      'avg(rag_search_duration_seconds_sum / rag_search_duration_seconds_count)',
      range,
    ),
    queryPrometheusRange('sum(rate(rag_search_total[1m])) * 60', range),
  ]);

  const timestamps = successRateSeries.map(([ts]) => ts);

  return {
    timestamps,
    successRate: successRateSeries.map(([, v]) => v),
    avgDuration: durationSeries.map(([, v]) => v),
    searchCount: countSeries.map(([, v]) => v),
  };
}
