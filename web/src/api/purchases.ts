/**
 * 采购管理 API
 */
import { get, post } from './client';
import type {
  PurchaseOrder,
  PurchaseTransitionRequest,
  PurchaseStatusLog,
  PageResponse,
  PageParams,
  Supplier,
} from '@/types/api';

export async function listPurchases(
  params: PageParams & { status?: string } = {},
): Promise<PageResponse<PurchaseOrder>> {
  return get<PageResponse<PurchaseOrder>>('/purchases/orders', params as Record<string, unknown>);
}

export async function getPurchase(id: number): Promise<PurchaseOrder> {
  return get<PurchaseOrder>(`/purchases/orders/${id}`);
}

export async function transitionPurchase(
  id: number,
  payload: PurchaseTransitionRequest,
): Promise<PurchaseOrder> {
  return post<PurchaseOrder>(
    `/purchases/orders/${id}/transition`,
    payload as unknown as Record<string, unknown>,
  );
}

export async function listPurchaseStatusLogs(id: number): Promise<PurchaseStatusLog[]> {
  return get<PurchaseStatusLog[]>(`/purchases/orders/${id}/logs`);
}

export interface CreatePurchasePayload {
  title?: string;
  product_id?: number;
  sku?: string;
  product_name: string;
  spec?: string;
  supplier_id: number;
  quantity: number;
  unit_price: number;
  currency?: string;
  payment_terms?: string;
  expected_date?: string;
  remark?: string;
}

export async function createPurchase(payload: CreatePurchasePayload): Promise<PurchaseOrder> {
  return post<PurchaseOrder>('/purchases/orders', payload);
}

export async function listSuppliers(params: PageParams = {}): Promise<PageResponse<Supplier>> {
  return get<PageResponse<Supplier>>('/suppliers', params as Record<string, unknown>);
}
