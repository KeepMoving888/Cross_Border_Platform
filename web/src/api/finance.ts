/**
 * 对账与利润 API
 */
import { get, post } from './client';
import type {
  FinanceBill,
  BillStatus,
  ProfitSummary,
  ProfitBySku,
  PageResponse,
  PageParams,
} from '@/types/api';

export async function listBills(
  params: PageParams & { status?: BillStatus; supplier_id?: number } = {},
): Promise<PageResponse<FinanceBill>> {
  return get<PageResponse<FinanceBill>>('/finance/bills', params as Record<string, unknown>);
}

export async function matchBill(id: number): Promise<unknown> {
  return post(`/finance/bills/${id}/match`);
}

export async function payBill(id: number, payload: { paid_amount: number; remark?: string }): Promise<FinanceBill> {
  return post<FinanceBill>(`/finance/bills/${id}/pay`, payload);
}

export async function getProfitSummary(): Promise<ProfitSummary> {
  return get<ProfitSummary>('/finance/profit/summary');
}

export async function getProfitBySku(): Promise<PageResponse<ProfitBySku>> {
  return get<PageResponse<ProfitBySku>>('/finance/profit/by-sku');
}
