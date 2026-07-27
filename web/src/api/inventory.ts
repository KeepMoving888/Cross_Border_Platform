/**
 * 库存管理 API
 */
import { get, post } from './client';
import type { Inventory, InventoryAlert, PageResponse, PageParams, Warehouse } from '@/types/api';

export async function listInventory(
  params: PageParams & { warehouse_id?: number; sku?: string } = {},
): Promise<PageResponse<Inventory>> {
  return get<PageResponse<Inventory>>('/inventory', params as Record<string, unknown>);
}

export async function listInventoryAlerts(
  params: PageParams & { status?: string; type?: string } = {},
): Promise<PageResponse<InventoryAlert>> {
  return get<PageResponse<InventoryAlert>>('/inventory/alerts', params as Record<string, unknown>);
}

export async function resolveInventoryAlert(id: number): Promise<void> {
  await post(`/inventory/alerts/${id}/resolve`);
}

export async function adjustInventory(payload: {
  warehouse_id: number;
  sku: string;
  quantity: number;
  type: 'inbound' | 'outbound' | 'adjust' | 'return';
  remark?: string;
}): Promise<Inventory> {
  return post<Inventory>('/inventory/adjust', payload);
}

export async function listWarehouses(): Promise<Warehouse[]> {
  return get<Warehouse[]>('/inventory/warehouses');
}
