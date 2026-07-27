/**
 * CB-Platform 统一 API 类型定义
 * 行业:家电美容(个护电器 + 美容仪器 + 美体仪器 + 配件耗材)
 */

/** 统一响应结构 */
export interface ApiResponse<T = unknown> {
  code: number;
  message: string;
  data: T;
  trace_id: string;
}

/** 统一分页响应 */
export interface PageResponse<T = unknown> {
  list: T[];
  total: number;
  page: number;
  page_size: number;
}

/** 分页查询参数 */
export interface PageParams {
  page?: number;
  page_size?: number;
  keyword?: string;
}

/** 登录用户信息 */
export interface UserInfo {
  id: number;
  username: string;
  nickname: string;
  avatar?: string;
  email?: string;
  role?: string;
  roles: string[];
  department?: string;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: UserInfo;
  user_info?: UserInfo;
}

/** 站内消息 */
export type MessageType = 'system' | 'stock_alert' | 'purchase' | 'finance' | 'ai';
export type MessageLevel = 'info' | 'warning' | 'critical' | 'success';

export interface MessageItem {
  id: number;
  user_id: number;
  type: MessageType | string;
  level: MessageLevel | string;
  title: string;
  content: string;
  ref_type?: string;
  ref_id?: string;
  link?: string;
  is_read: boolean;
  created_at?: string;
}

/** 仓库 */
export interface Warehouse {
  id: number;
  code: string;
  name: string;
  type: string;
  country?: string;
  address?: string;
  manager?: string;
  status?: number;
}

/** 类目枚举(家电美容) */
export type ProductCategory =
  | 'personal_care'
  | 'beauty_device'
  | 'body_shaping'
  | 'accessories';

/** 选品阶段枚举(对齐后端 models.ProductStage*) */
export type ProductStage =
  | 'sourcing'
  | 'testing'
  | 'approved'
  | 'rejected'
  | 'archived';

export type Platform = 'amazon' | 'temu' | 'tiktok' | 'shopify';

export type TargetMarket = 'US' | 'EU' | 'JP' | 'SEA' | 'ME' | 'AU';

/** 选品 SPU */
export interface Product {
  id: number;
  sku: string;
  asin?: string;
  name: string;
  category: ProductCategory;
  sub_category: string;
  stage: ProductStage;
  platform: Platform;
  target_market: TargetMarket;
  list_price: number;
  est_cost_price: number;
  est_margin_rate: number;
  ai_score: number;
  monthly_sales: number;
  review_count: number;
  rating: number;
  tags: string[];
  ai_insight: string;
}

export interface ProductQuery extends PageParams {
  category?: ProductCategory;
  stage?: ProductStage;
  platform?: Platform;
  sort_by?: 'ai_score' | 'monthly_sales' | 'rating' | 'est_margin_rate';
  sort_order?: 'asc' | 'desc';
}

/** 商品市场趋势(对齐后端 ProductTrend) */
export interface ProductTrend {
  id: number;
  product_id: number;
  sku: string;
  stat_date: string;
  platform: string;
  market: string;
  search_volume: number;
  sales_volume: number;
  competitor_count: number;
  avg_price: number;
  review_growth: number;
}

/** 竞品监控(对齐后端 ProductCompetitor) */
export interface ProductCompetitor {
  id: number;
  product_id: number;
  competitor_asin: string;
  competitor_sku: string;
  brand: string;
  price: number;
  sales_est: number;
  review_count: number;
  rating: number;
  listing_url?: string;
}

/** 供应商 */
export type SupplierCoopStatus = 'active' | 'pending' | 'frozen' | 'terminated';

export interface Supplier {
  id: number;
  name: string;
  code: string;
  contact_name: string;
  phone: string;
  region: string;
  rating: number;
  coop_status: SupplierCoopStatus;
  total_amount: number;
  on_time_rate: number;
  quality_rate: number;
}

/** 采购单状态 */
export type PurchaseStatus =
  | 'inquiry'
  | 'quoting'
  | 'ordered'
  | 'tracking'
  | 'shipped'
  | 'received'
  | 'qc'
  | 'reconciling'
  | 'settled'
  | 'cancelled';

export interface PurchaseOrder {
  id: number;
  order_no: string;
  title: string;
  product_name: string;
  sku: string;
  supplier_id: number;
  supplier_name?: string;
  quantity: number;
  unit_price: number;
  total_amount: number;
  status: PurchaseStatus;
  expected_date: string;
  actual_date?: string;
  logistics_no?: string;
  logistics_company?: string;
}

export interface PurchaseTransitionRequest {
  event: string;
  remark?: string;
}

/** 采购单状态变更日志(对应后端 PurchaseStatusLog) */
export interface PurchaseStatusLog {
  id: number;
  order_id: number;
  from_status: string;
  to_status: string;
  operator_id?: number;
  operator_name?: string;
  remark?: string;
  created_at: string;
  updated_at: string;
}

/** 采购单状态机:状态 -> 可执行事件列表 */
export interface StatusEventOption {
  event: string;
  label: string;
  next_status: PurchaseStatus;
}

/** 库存 */
export interface Inventory {
  id: number;
  warehouse_id: number;
  warehouse_name?: string;
  sku: string;
  product_name?: string;
  available_qty: number;
  locked_qty: number;
  in_transit_qty: number;
  safety_stock: number;
  unit_cost: number;
}

export interface InventoryAlert {
  id: number;
  sku: string;
  product_name?: string;
  warehouse_id: number;
  warehouse_name?: string;
  /** 后端字段:当前库存 */
  current_qty: number;
  /** 后端字段:阈值(=安全库存) */
  threshold: number;
  /** 后端字段:预警类型 out_of_stock / low_stock */
  type: string;
  /** 后端字段:预警级别 pending / resolved */
  status: string;
  // 兼容字段(前端历史使用)
  available_qty?: number;
  safety_stock?: number;
  shortage_qty?: number;
  level?: 'warning' | 'critical';
  created_at?: string;
}

/** 财务账单 */
export type BillStatus = 'draft' | 'matching' | 'matched' | 'disputed' | 'paid';

export interface FinanceBill {
  id: number;
  bill_no: string;
  order_no: string;
  supplier_id: number;
  supplier_name?: string;
  payable_amount: number;
  paid_amount: number;
  diff_amount: number;
  status: BillStatus;
  matched_at?: string;
  paid_at?: string;
}

/** 利润汇总(对齐后端 /finance/profit/summary 返回) */
export interface ProfitSummary {
  total_revenue: number;
  total_goods_cost: number;
  total_freight_cost: number;
  total_platform_fee: number;
  total_ad_cost: number;
  total_tax_cost: number;
  total_refund_cost: number;
  total_other_cost: number;
  total_net_profit: number;
  order_count: number;
  avg_margin_rate: number; // 已乘 100,如 33.4 表示 33.4%
}

/** SKU 利润(对齐后端 /finance/profit/by-sku 列表项) */
export interface ProfitBySku {
  sku: string;
  revenue: number;
  goods_cost: number;
  freight_cost: number;
  platform_fee: number;
  ad_cost: number;
  net_profit: number;
  margin_rate: number; // 已乘 100
  order_count: number;
}

/** AI 工作流 */
/** AI 工作流类型枚举(对齐后端 defaultAIWorkflows().Type) */
export type AIWorkflowType = 'agent' | 'automation' | 'rag' | 'text2sql';
export type AIWorkflowStatus = 'enabled' | 'disabled' | 'running';

export interface AIWorkflow {
  id: number;
  code: string;
  name: string;
  description: string;
  type: AIWorkflowType;
  scene: string;
  status: AIWorkflowStatus;
}

export interface AIWorkflowRunRequest {
  input: Record<string, string | number | boolean>;
}

export interface AIWorkflowRunResult {
  workflow_id: number;
  workflow_code?: string;
  run_id: string;
  status: 'success' | 'failed' | 'partial';
  output: string;
  error?: string;
  metrics?: Record<string, number | string>;
  duration_ms: number;
  finished_at: string;
}

/** AI 工作流执行记录(历史列表用) */
export interface AIWorkflowRun {
  id: number;
  workflow_id: number;
  workflow_code: string;
  trigger_type: string;
  input: string;
  output: string;
  status: 'running' | 'success' | 'failed' | 'timeout';
  error?: string;
  duration_ms: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost: number | string;
  operator_id?: number;
  ref_type?: string;
  ref_id?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

/** 工作台总览(对齐后端 overviewResp) */
export interface DashboardOverview {
  // 销售 & 利润
  today_sales: number;
  yesterday_sales: number;
  month_sales: number;
  net_profit: number;
  total_revenue: number;
  month_profit: number;
  margin_rate: number;
  order_count_30d: number;
  refund_amount: number;
  // 选品
  product_total: number;
  product_approved: number;
  product_sourcing: number;
  new_products_7d: number;
  // 采购
  pending_purchase_orders: number;
  purchase_total: number;
  // 供应商
  supplier_active: number;
  // 库存
  inventory_skus: number;
  inventory_alerts: number;
  // 财务对账
  bills_pending: number;
  // AI
  ai_runs_today: number;
  ai_runs_total: number;
  ai_running_tasks: number;
  ai_success_rate: number;
}

export interface SalesTrendPoint {
  date: string;
  sales: number;
  orders: number;
  net_profit: number;
  refund_amount: number;
}

export interface CategorySharePoint {
  category: ProductCategory;
  category_name: string;
  sales: number;
  share: number;
}

export interface ProductStats {
  total: number;
  by_stage: Array<{ stage: ProductStage; count: number }>;
  by_category: Array<{ category: ProductCategory; count: number; avg_ai_score: number }>;
}

/** 利润统计(对齐后端 /dashboard/profit/stats 实际返回) */
export interface ProfitStats {
  cost_breakdown: {
    goods_cost: number | string;
    freight_cost: number | string;
    platform_fee: number | string;
    ad_cost: number | string;
    tax_cost: number | string;
    refund_cost: number | string;
    other_cost: number | string;
  };
  by_month: Array<{
    month: string;
    revenue: number | string;
    net_profit: number | string;
    margin_rate: number | string;
  }>;
}

/** 平台对接 */
export type PlatformAccountStatus = 'connected' | 'disconnected' | 'expired' | 'syncing';

export interface PlatformAccount {
  id: number;
  platform: Platform;
  name: string;
  account_id: string;
  region: string;
  status: PlatformAccountStatus;
  last_sync_at?: string;
  product_count: number;
  order_count: number;
  sales_30d: number;
}

/** 通用 ID 标识 */
export interface IdParam {
  id: number;
}
