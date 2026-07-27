/**
 * 认证相关 API(直接对接后端)
 */
import { post } from './client';
import type { LoginRequest, LoginResponse } from '@/types/api';

export async function login(payload: LoginRequest): Promise<LoginResponse> {
  return post<LoginResponse>('/auth/login', payload as unknown as Record<string, unknown>);
}

export async function logout(): Promise<void> {
  try {
    await post('/auth/logout');
  } catch {
    // 静默处理:登出失败不阻塞前端流程
  }
}
