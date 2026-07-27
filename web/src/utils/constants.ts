/**
 * 状态枚举、颜色映射、状态机定义等行业常量
 */
import type {
  ProductCategory,
  ProductStage,
  Platform,
  TargetMarket,
  PurchaseStatus,
  BillStatus,
  SupplierCoopStatus,
  PlatformAccountStatus,
  AIWorkflowType,
  AIWorkflowStatus,
  StatusEventOption,
} from '@/types/api';

/** Antd Tag/Badge 支持的颜色名 */
export type PresetColor =
  | 'success'
  | 'processing'
  | 'error'
  | 'default'
  | 'warning'
  | 'magenta'
  | 'red'
  | 'volcano'
  | 'orange'
  | 'gold'
  | 'lime'
  | 'green'
  | 'cyan'
  | 'blue'
  | 'geekblue'
  | 'purple';

/** 类目字典(家电美容) */
export const PRODUCT_CATEGORY_MAP: Record<ProductCategory, { label: string; color: PresetColor }> = {
  personal_care: { label: '个护电器', color: 'blue' },
  beauty_device: { label: '美容仪器', color: 'purple' },
  body_shaping: { label: '美体仪器', color: 'magenta' },
  accessories: { label: '配件耗材', color: 'gold' },
};

/** 选品阶段字典(对齐后端 models.ProductStage*) */
export const PRODUCT_STAGE_MAP: Record<ProductStage, { label: string; color: PresetColor }> = {
  sourcing: { label: '寻源中', color: 'processing' },
  testing: { label: '测试中', color: 'gold' },
  approved: { label: '已通过', color: 'success' },
  rejected: { label: '已否决', color: 'volcano' },
  archived: { label: '已归档', color: 'default' },
};

/** 平台字典 */
export const PLATFORM_MAP: Record<Platform, { label: string; color: PresetColor }> = {
  amazon: { label: '亚马逊', color: 'orange' },
  temu: { label: 'Temu', color: 'red' },
  tiktok: { label: 'TikTok', color: 'cyan' },
  shopify: { label: 'Shopify', color: 'green' },
};

/** 目标市场字典 */
export const TARGET_MARKET_MAP: Record<TargetMarket, string> = {
  US: '北美',
  EU: '欧洲',
  JP: '日本',
  SEA: '东南亚',
  ME: '中东',
  AU: '澳洲',
};

/** 采购单状态字典 */
export const PURCHASE_STATUS_MAP: Record<PurchaseStatus, { label: string; color: PresetColor }> = {
  inquiry: { label: '询价中', color: 'default' },
  quoting: { label: '报价中', color: 'processing' },
  ordered: { label: '已下单', color: 'blue' },
  tracking: { label: '追踪中', color: 'cyan' },
  shipped: { label: '已发货', color: 'gold' },
  received: { label: '已到货', color: 'geekblue' },
  qc: { label: '质检中', color: 'purple' },
  reconciling: { label: '对账中', color: 'magenta' },
  settled: { label: '已结算', color: 'success' },
  cancelled: { label: '已取消', color: 'error' },
};

/** 采购状态机:每个状态可触发的流转事件 */
export const PURCHASE_TRANSITIONS: Record<PurchaseStatus, StatusEventOption[]> = {
  inquiry: [{ event: 'to_quote', label: '发起报价', next_status: 'quoting' }],
  quoting: [
    { event: 'to_order', label: '确认下单', next_status: 'ordered' },
    { event: 'cancel', label: '取消询价', next_status: 'cancelled' },
  ],
  ordered: [
    { event: 'to_tracking', label: '开始追踪', next_status: 'tracking' },
    { event: 'cancel', label: '取消订单', next_status: 'cancelled' },
  ],
  tracking: [{ event: 'to_shipped', label: '确认发货', next_status: 'shipped' }],
  shipped: [{ event: 'to_received', label: '确认到货', next_status: 'received' }],
  received: [{ event: 'to_qc', label: '进入质检', next_status: 'qc' }],
  qc: [
    { event: 'pass_qc', label: '质检通过', next_status: 'reconciling' },
    { event: 'fail_qc', label: '质检不通过', next_status: 'received' },
  ],
  reconciling: [{ event: 'to_settled', label: '完成结算', next_status: 'settled' }],
  settled: [],
  cancelled: [],
};

/** 账单状态字典 */
export const BILL_STATUS_MAP: Record<BillStatus, { label: string; color: PresetColor }> = {
  draft: { label: '待对账', color: 'default' },
  matching: { label: '对账中', color: 'processing' },
  matched: { label: '已对平', color: 'success' },
  disputed: { label: '有争议', color: 'error' },
  paid: { label: '已付款', color: 'geekblue' },
};

/** 供应商合作状态字典 */
export const SUPPLIER_COOP_STATUS_MAP: Record<SupplierCoopStatus, { label: string; color: PresetColor }> = {
  active: { label: '合作中', color: 'success' },
  pending: { label: '待审核', color: 'processing' },
  frozen: { label: '已冻结', color: 'warning' },
  terminated: { label: '已终止', color: 'error' },
};

/** 平台账号状态字典 */
export const PLATFORM_ACCOUNT_STATUS_MAP: Record<PlatformAccountStatus, { label: string; color: PresetColor }> = {
  connected: { label: '已连接', color: 'success' },
  disconnected: { label: '未连接', color: 'default' },
  expired: { label: '已过期', color: 'error' },
  syncing: { label: '同步中', color: 'processing' },
};

/** AI 工作流类型字典(对齐后端 defaultAIWorkflows().Type) */
export const AI_WORKFLOW_TYPE_MAP: Record<AIWorkflowType, { label: string; color: PresetColor }> = {
  agent: { label: '智能体', color: 'blue' },
  automation: { label: '自动化', color: 'green' },
  rag: { label: 'RAG 检索', color: 'purple' },
  text2sql: { label: 'Text2SQL', color: 'magenta' },
};

export const AI_WORKFLOW_STATUS_MAP: Record<AIWorkflowStatus, { label: string; color: PresetColor }> = {
  enabled: { label: '已启用', color: 'success' },
  disabled: { label: '已停用', color: 'default' },
  running: { label: '运行中', color: 'processing' },
};

/** 仓库字典(对齐 seed: 深圳/美西/欧洲/FBA/FBA-EU) */
export const WAREHOUSE_MAP: Record<number, string> = {
  1: '深圳主仓',
  2: '美西海外仓',
  3: '欧洲海外仓',
  4: '亚马逊 FBA 仓',
  5: '亚马逊 FBA 欧洲仓',
};

/** 角色显示名 */
export const ROLE_LABEL_MAP: Record<string, string> = {
  admin: '系统管理员',
  manager: '业务经理',
  staff: '业务专员',
};

/** 类目选项(供筛选下拉) */
export const CATEGORY_OPTIONS = Object.entries(PRODUCT_CATEGORY_MAP).map(([value, { label }]) => ({
  value: value as ProductCategory,
  label,
}));

export const STAGE_OPTIONS = Object.entries(PRODUCT_STAGE_MAP).map(([value, { label }]) => ({
  value: value as ProductStage,
  label,
}));

export const PLATFORM_OPTIONS = Object.entries(PLATFORM_MAP).map(([value, { label }]) => ({
  value: value as Platform,
  label,
}));

/** 默认分页参数 */
export const DEFAULT_PAGE_SIZE = 10;
export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

/** Token 本地存储键 */
export const TOKEN_STORAGE_KEY = 'cbp_token';
export const USER_STORAGE_KEY = 'cbp_user';
