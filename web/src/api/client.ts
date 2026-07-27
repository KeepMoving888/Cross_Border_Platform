/**
 * Axios 实例与拦截器
 * - 请求自动注入 Bearer Token
 * - 响应自动解包 data 字段
 * - 401 自动跳转登录
 */
import axios, { AxiosError, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { message } from 'antd';
import { TOKEN_STORAGE_KEY } from '@/utils/constants';
import type { ApiResponse } from '@/types/api';

export const http = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
  withCredentials: false,
});

/** 请求拦截:注入 Token */
http.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem(TOKEN_STORAGE_KEY);
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error),
);

/** 响应拦截:统一解包 ApiResponse.data */
http.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => {
    const body = response.data;
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code === 0) {
        return body.data as unknown as AxiosResponse;
      }
      message.error(body.message || `请求失败(code=${body.code})`);
      return Promise.reject(new Error(body.message || 'business_error'));
    }
    return response;
  },
  (error: AxiosError<ApiResponse>) => {
    if (error.response) {
      const { status, data } = error.response;
      if (status === 401) {
        localStorage.removeItem(TOKEN_STORAGE_KEY);
        const cur = window.location.pathname;
        if (cur !== '/login') {
          message.warning('登录状态已过期,请重新登录');
          window.location.href = '/login';
        }
        return Promise.reject(error);
      }
      const msg = data?.message || `请求异常(${status})`;
      message.error(msg);
    } else if (error.request) {
      message.error('网络异常,后端服务可能未启动');
    } else {
      message.error(error.message || '未知错误');
    }
    return Promise.reject(error);
  },
);

/**
 * 通用 GET 请求,带类型推断
 */
export async function get<T>(url: string, params?: Record<string, unknown>): Promise<T> {
  const res = await http.get<T>(url, { params });
  return res as unknown as T;
}

/** 通用 POST 请求 */
export async function post<T>(url: string, body?: unknown): Promise<T> {
  const res = await http.post<T>(url, body);
  return res as unknown as T;
}

/** 通用 PUT 请求 */
export async function put<T>(url: string, body?: unknown): Promise<T> {
  const res = await http.put<T>(url, body);
  return res as unknown as T;
}

/** 通用 DELETE 请求 */
export async function del<T>(url: string): Promise<T> {
  const res = await http.delete<T>(url);
  return res as unknown as T;
}
