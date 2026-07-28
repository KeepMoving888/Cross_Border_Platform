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
