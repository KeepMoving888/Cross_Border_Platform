/**
 * 平台对接:亚马逊 / Temu / TikTok 账号卡片 + 同步数据
 */
import React, { useEffect, useState } from 'react';
import {
  Card,
  Row,
  Col,
  Button,
  Tag,
  Space,
  Typography,
  Spin,
  Empty,
  Statistic,
  Tooltip,
  message,
} from 'antd';
import {
  SyncOutlined,
  AmazonOutlined,
  ShoppingOutlined,
  VideoCameraOutlined,
  ShopOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { listPlatforms, syncPlatform } from '@/api/platforms';
import { PLATFORM_MAP, PLATFORM_ACCOUNT_STATUS_MAP } from '@/utils/constants';
import { formatUSD, formatNumber, formatRelativeTime } from '@/utils/format';
import type { PlatformAccount, Platform } from '@/types/api';

const { Text, Title } = Typography;

const PLATFORM_ICON: Record<Platform, React.ReactNode> = {
  amazon: <AmazonOutlined />,
  temu: <ShoppingOutlined />,
  tiktok: <VideoCameraOutlined />,
  shopify: <ShopOutlined />,
};

const PLATFORM_BRAND_COLOR: Record<Platform, string> = {
  amazon: '#FF9900',
  temu: '#FF4747',
  tiktok: '#06D6E0',
  shopify: '#95BF47',
};

const Platforms: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [accounts, setAccounts] = useState<PlatformAccount[]>([]);
  const [syncing, setSyncing] = useState<Record<number, boolean>>({});

  const reload = async () => {
    setLoading(true);
    try {
      const data = await listPlatforms();
      setAccounts(data);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    reload();
  }, []);

  const handleSync = async (acc: PlatformAccount) => {
    if (acc.status === 'expired') {
      message.warning('该账号授权已过期,请先重新授权后再同步');
      return;
    }
    setSyncing((s) => ({ ...s, [acc.id]: true }));
    try {
      const res = await syncPlatform(acc.id);
      message.success(res.message || '同步任务已提交');
      // 局部更新状态为 syncing
      setAccounts((list) =>
        list.map((a) => (a.id === acc.id ? { ...a, status: 'syncing', last_sync_at: new Date().toISOString() } : a)),
      );
    } catch (err) {
      // 错误已由拦截器提示
    } finally {
      setSyncing((s) => ({ ...s, [acc.id]: false }));
    }
  };

  return (
    <PageContainer
      title="平台对接"
      breadcrumb={{}}
      extra={[
        <Button key="reload" icon={<ReloadOutlined />} onClick={reload}>
          刷新
        </Button>,
      ]}
    >
      <Spin spinning={loading}>
        {accounts.length === 0 && !loading ? (
          <Empty description="暂无已对接平台账号" />
        ) : (
          <Row gutter={[16, 16]}>
            {accounts.map((acc) => {
              const brandColor = PLATFORM_BRAND_COLOR[acc.platform];
              const isSyncing = syncing[acc.id] || acc.status === 'syncing';
              return (
                <Col xs={24} sm={12} xl={8} key={acc.id}>
                  <Card
                    className="cbp-platform-card"
                    bordered
                    style={{ borderRadius: 12, height: '100%' }}
                    styles={{ body: { padding: 20 } }}
                  >
                    <Space align="start" style={{ width: '100%' }}>
                      <div
                        style={{
                          width: 52,
                          height: 52,
                          borderRadius: 12,
                          background: `${brandColor}1a`,
                          color: brandColor,
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: 26,
                          flexShrink: 0,
                        }}
                      >
                        {PLATFORM_ICON[acc.platform]}
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <Title level={5} style={{ margin: 0, fontWeight: 600 }}>
                          {acc.name}
                        </Title>
                        <Space size={6} style={{ marginTop: 4 }}>
                          <StatusTag status={acc.platform} map={PLATFORM_MAP} />
                          <StatusTag status={acc.status} map={PLATFORM_ACCOUNT_STATUS_MAP} />
                        </Space>
                      </div>
                    </Space>

                    <Row gutter={[12, 12]} style={{ marginTop: 18 }}>
                      <Col span={12}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          账号 ID
                        </Text>
                        <div style={{ fontSize: 13, marginTop: 2 }}>
                          <Text copyable style={{ fontSize: 13 }}>
                            {acc.account_id}
                          </Text>
                        </div>
                      </Col>
                      <Col span={12}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          区域
                        </Text>
                        <div style={{ fontSize: 13, marginTop: 2 }}>{acc.region}</div>
                      </Col>
                      <Col span={12}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          上架商品
                        </Text>
                        <div style={{ fontSize: 13, marginTop: 2 }}>{formatNumber(acc.product_count)} 个</div>
                      </Col>
                      <Col span={12}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          近 30 天订单
                        </Text>
                        <div style={{ fontSize: 13, marginTop: 2 }}>{formatNumber(acc.order_count)} 单</div>
                      </Col>
                      <Col span={24}>
                        <Statistic
                          title="近 30 天销售额"
                          value={acc.sales_30d}
                          precision={2}
                          prefix="$"
                          valueStyle={{ fontSize: 20, color: brandColor, fontWeight: 600 }}
                          formatter={(v) => formatNumber(Number(v))}
                        />
                      </Col>
                      <Col span={24}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          最近同步:
                        </Text>{' '}
                        <Text style={{ fontSize: 12 }}>
                          {acc.last_sync_at ? formatRelativeTime(acc.last_sync_at) : '尚未同步'}
                        </Text>
                      </Col>
                    </Row>

                    <Space style={{ marginTop: 16, width: '100%', justifyContent: 'flex-end' }}>
                      <Tooltip title={acc.status === 'expired' ? '授权已过期' : '从平台拉取最新订单与库存数据'}>
                        <Button
                          type="primary"
                          icon={isSyncing ? <SyncOutlined spin /> : <SyncOutlined />}
                          onClick={() => handleSync(acc)}
                          loading={isSyncing}
                          disabled={acc.status === 'expired'}
                          style={{
                            background: `linear-gradient(90deg, ${brandColor} 0%, ${brandColor}cc 100%)`,
                            border: 'none',
                          }}
                        >
                          {isSyncing ? '同步中' : '同步数据'}
                        </Button>
                      </Tooltip>
                    </Space>
                  </Card>
                </Col>
              );
            })}
          </Row>
        )}
      </Spin>
    </PageContainer>
  );
};

export default Platforms;
