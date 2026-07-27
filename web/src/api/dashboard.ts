/**
 * 工作台 / 数据看板 API(直接对接后端 /api/v1/dashboard/*)
 */
import { get } from './client';
import type {
  DashboardOverview,
  SalesTrendPoint,
  CategorySharePoint,
  ProductStats,
  ProfitStats,
} from '@/types/api';

/** AI 使用统计(对齐后端 /dashboard/ai/stats 返回) */
export interface AIStatsScene {
  workflow_code: string;
  count: number;
  success_count: number;
  tokens: number;
  cost: number;
  avg_duration_ms: number;
}

export interface AIStatsResp {
  by_scene: AIStatsScene[];
  by_day: Array<{ date: string; count: number }>;
}

export async function getOverview(): Promise<DashboardOverview> {
  return get<DashboardOverview>('/dashboard/overview');
}

export async function getSalesTrend(days = 30): Promise<SalesTrendPoint[]> {
  return get<SalesTrendPoint[]>('/dashboard/sales-trend', { days });
}

export async function getCategoryShare(): Promise<CategorySharePoint[]> {
  return get<CategorySharePoint[]>('/dashboard/category-share');
}

export async function getProductStats(): Promise<ProductStats> {
  return get<ProductStats>('/dashboard/product/stats');
}

export async function getProfitStats(): Promise<ProfitStats> {
  return get<ProfitStats>('/dashboard/profit/stats');
}

export async function getAIStats(): Promise<AIStatsResp> {
  return get<AIStatsResp>('/dashboard/ai/stats');
}

/** 利润统计(对齐后端 /dashboard/profit/stats) */
export interface ProfitStatsResp {
  cost_breakdown: {
    goods_cost: number;
    freight_cost: number;
    platform_fee: number;
    ad_cost: number;
    tax_cost: number;
    refund_cost: number;
    other_cost: number;
  };
  by_month: Array<{
    month: string;
    revenue: number;
    net_profit: number;
    margin_rate: number;
  }>;
}

export async function getDashboardProfitStats(): Promise<ProfitStatsResp> {
  return get<ProfitStatsResp>('/dashboard/profit/stats');
}
