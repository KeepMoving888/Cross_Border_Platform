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
  PageResponse,
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
