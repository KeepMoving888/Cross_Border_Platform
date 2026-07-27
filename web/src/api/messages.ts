/**
 * 消息中心 API
 */
import { get, post, put } from './client';
import type { PageResponse, PageParams, MessageItem } from '@/types/api';

export async function listMessages(
  params: PageParams & { is_read?: boolean; type?: string } = {},
): Promise<PageResponse<MessageItem>> {
  return get<PageResponse<MessageItem>>('/messages', params as Record<string, unknown>);
}

export async function getUnreadCount(): Promise<{ count: number }> {
  return get<{ count: number }>('/messages/unread-count');
}

export async function markMessageRead(id: number): Promise<void> {
  await put(`/messages/${id}/read`);
}

export async function markAllMessagesRead(): Promise<void> {
  await post('/messages/read-all');
}
