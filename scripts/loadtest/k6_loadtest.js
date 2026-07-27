// k6 负载测试脚本
//
// 安装 k6: https://k6.io/docs/get-started/installation/
// 运行: k6 run scripts/loadtest/k6_loadtest.js
//
// 测试场景:
//   1. 健康检查端点(高频)
//   2. 登录接口(中频)
//   3. 列表查询接口(中频)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// 自定义指标
const errorRate = new Rate('errors');
const healthLatency = new Trend('health_latency');
const loginLatency = new Trend('login_latency');

// 测试配置
export const options = {
  stages: [
    { duration: '30s', target: 20 },   // 30 秒内爬升到 20 并发
    { duration: '1m', target: 50 },    // 1 分钟内爬升到 50 并发
    { duration: '2m', target: 50 },    // 维持 50 并发 2 分钟
    { duration: '30s', target: 0 },    // 30 秒内降到 0
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],  // 95% 请求 < 500ms,99% < 1s
    http_req_failed: ['rate<0.01'],                   // 错误率 < 1%
    errors: ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// 测试账号(从种子数据初始化)
const TEST_USERNAME = 'admin';
const TEST_PASSWORD = 'admin123';

export default function () {
  // 1. 健康检查(每个迭代都执行)
  const healthRes = http.get(`${BASE_URL}/health`);
  healthLatency.add(healthRes.timings.duration);
  check(healthRes, {
    'health status 200': (r) => r.status === 200,
    'health body ok': (r) => r.json('status') === 'ok',
  });
  errorRate.add(healthRes.status !== 200);

  // 2. 登录测试
  const loginRes = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({
      username: TEST_USERNAME,
      password: TEST_PASSWORD,
    }),
    {
      headers: { 'Content-Type': 'application/json' },
    }
  );
  loginLatency.add(loginRes.timings.duration);

  const loginSuccess = check(loginRes, {
    'login status 200': (r) => r.status === 200,
    'login has token': (r) => r.json('data') && r.json('data').token,
  });
  errorRate.add(!loginSuccess);

  // 3. 使用 token 访问受保护资源
  if (loginSuccess) {
    const token = loginRes.json('data').token;
    const authRes = http.get(`${BASE_URL}/api/v1/dashboard/overview`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    check(authRes, {
      'dashboard status 200': (r) => r.status === 200,
    });
    errorRate.add(authRes.status !== 200);
  }

  sleep(0.5);
}

// 测试结束汇总
export function handleSummary(data) {
  return {
    stdout: textSummary(data),
    'scripts/loadtest/results.json': JSON.stringify(data, null, 2),
  };
}

function textSummary(data) {
  return `
=== CB-Platform 压测结果 ===

测试时长: ${data.metrics.iterations ? data.metrics.iterations.values.duration : 'N/A'}s
总请求数: ${data.metrics.http_reqs ? data.metrics.http_reqs.values.count : 0}
实际 QPS: ${data.metrics.http_reqs ? data.metrics.http_reqs.values.rate.toFixed(2) : 0}

响应时间:
  平均: ${data.metrics.http_req_duration ? data.metrics.http_req_duration.values.avg.toFixed(2) : 0}ms
  P50:  ${data.metrics.http_req_duration ? data.metrics.http_req_duration.values['p(50)'].toFixed(2) : 0}ms
  P90:  ${data.metrics.http_req_duration ? data.metrics.http_req_duration.values['p(90)'].toFixed(2) : 0}ms
  P95:  ${data.metrics.http_req_duration ? data.metrics.http_req_duration.values['p(95)'].toFixed(2) : 0}ms
  P99:  ${data.metrics.http_req_duration ? data.metrics.http_req_duration.values['p(99)'].toFixed(2) : 0}ms

错误率: ${data.metrics.http_req_failed ? (data.metrics.http_req_failed.values.rate * 100).toFixed(2) : 0}%

阈值检查: ${data.metrics.http_req_failed && data.metrics.http_req_failed.values.rate < 0.01 ? 'PASS' : 'FAIL'}
`;
}
