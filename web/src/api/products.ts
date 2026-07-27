/**
 * 选品管理 API
 */
import { get, post, put, del } from './client';
import type {
  Product,
  ProductQuery,
  PageResponse,
  ProductTrend,
  ProductCompetitor,
  ProductStage,
} from '@/types/api';

export async function listProducts(params: ProductQuery = {}): Promise<PageResponse<Product>> {
  return get<PageResponse<Product>>('/products', params as Record<string, unknown>);
}

export async function getProduct(id: number): Promise<Product> {
  return get<Product>(`/products/${id}`);
}

export async function listProductTrends(id: number): Promise<ProductTrend[]> {
  return get<ProductTrend[]>(`/products/${id}/trends`);
}

export async function listProductCompetitors(id: number): Promise<ProductCompetitor[]> {
  return get<ProductCompetitor[]>(`/products/${id}/competitors`);
}

export interface CreateProductPayload {
  sku: string;
  name: string;
  asin?: string;
  category?: string;
  sub_category?: string;
  stage?: ProductStage;
  platform?: string;
  target_market?: string;
  list_price?: number;
  est_cost_price?: number;
  currency?: string;
  monthly_sales?: number;
  review_count?: number;
  rating?: number;
  tags?: string;
  remark?: string;
  supplier_id?: number;
}

export async function createProduct(payload: CreateProductPayload): Promise<Product> {
  return post<Product>('/products', payload);
}

export async function updateProduct(id: number, payload: Partial<CreateProductPayload>): Promise<Product> {
  return put<Product>(`/products/${id}`, payload);
}

export async function changeProductStage(
  id: number,
  payload: { stage: ProductStage; remark?: string },
): Promise<Product> {
  return put<Product>(`/products/${id}/stage`, payload);
}

export async function deleteProduct(id: number): Promise<void> {
  await del(`/products/${id}`);
}
