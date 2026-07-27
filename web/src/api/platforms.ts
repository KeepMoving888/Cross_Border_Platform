/**
 * 平台对接 API(直接对接后端)
 */
import { get, post } from './client';
import type { PlatformAccount } from '@/types/api';

export async function listPlatforms(): Promise<PlatformAccount[]> {
  return get<PlatformAccount[]>('/platform/accounts');
}

export async function syncPlatform(id: number): Promise<{ status: string; message: string }> {
  return post<{ status: string; message: string }>(`/platform/accounts/${id}/sync`);
}
