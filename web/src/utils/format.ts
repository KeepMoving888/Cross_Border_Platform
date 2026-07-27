/**
 * 金额、日期、百分比等通用格式化工具
 */
import dayjs, { Dayjs } from 'dayjs';

/** 千分位整数(兼容后端 decimal.Decimal 字符串) */
export function formatNumber(value: number | string | undefined | null, fallback = '-'): string {
  if (value === undefined || value === null || value === '') return fallback;
  const n = Number(value);
  if (Number.isNaN(n)) return fallback;
  return n.toLocaleString('zh-CN');
}

/** 金额(人民币 ¥,兼容后端 decimal.Decimal 字符串) */
export function formatCNY(value: number | string | undefined | null, fallback = '-'): string {
  if (value === undefined || value === null || value === '') return fallback;
  const n = Number(value);
  if (Number.isNaN(n)) return fallback;
  const sign = n < 0 ? '-' : '';
  return `${sign}¥${Math.abs(n).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

/** 金额(美元 $,兼容后端 decimal.Decimal 字符串) */
export function formatUSD(value: number | string | undefined | null, fallback = '-'): string {
  if (value === undefined || value === null || value === '') return fallback;
  const n = Number(value);
  if (Number.isNaN(n)) return fallback;
  const sign = n < 0 ? '-' : '';
  return `${sign}$${Math.abs(n).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

/** 百分比(传 0.12 表示 12%,兼容后端 decimal.Decimal 字符串) */
export function formatPercent(value: number | string | undefined | null, digits = 1, fallback = '-'): string {
  if (value === undefined || value === null || value === '') return fallback;
  const n = Number(value);
  if (Number.isNaN(n)) return fallback;
  return `${(n * 100).toFixed(digits)}%`;
}

/** 评分(保留 1 位小数,兼容字符串/数字) */
export function formatRating(value: number | string | undefined | null, fallback = '-'): string {
  if (value === undefined || value === null || value === '') return fallback;
  const n = Number(value);
  if (Number.isNaN(n)) return fallback;
  return n.toFixed(1);
}

/** 日期 YYYY-MM-DD */
export function formatDate(value: string | number | Date | Dayjs | undefined | null, fallback = '-'): string {
  if (!value) return fallback;
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD') : fallback;
}

/** 图表用短日期 MM-DD(兼容时间戳/RFC3339/YYYY-MM-DD) */
export function formatChartDate(value: string | number | Date | undefined | null, fallback = ''): string {
  if (value === undefined || value === null || value === '') return fallback;
  const raw = String(value);
  // 纯日期或带时间，统一 dayjs 解析
  const d = dayjs(raw.length >= 10 ? raw.slice(0, 10) : raw);
  if (d.isValid()) return d.format('MM-DD');
  if (raw.length >= 10) return raw.slice(5, 10);
  return raw;
}

/** 日期时间 YYYY-MM-DD HH:mm:ss */
export function formatDateTime(value: string | number | Date | Dayjs | undefined | null, fallback = '-'): string {
  if (!value) return fallback;
  const d = dayjs(value);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : fallback;
}

/** 相对时间(简化版) */
export function formatRelativeTime(value: string | number | Date | undefined | null, fallback = '-'): string {
  if (!value) return fallback;
  const d = dayjs(value);
  if (!d.isValid()) return fallback;
  const diff = Date.now() - d.valueOf();
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < minute) return '刚刚';
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < 30 * day) return `${Math.floor(diff / day)} 天前`;
  return d.format('YYYY-MM-DD');
}
