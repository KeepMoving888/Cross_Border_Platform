/**
 * ProLayout 主布局:企业级 SaaS 侧栏 + 顶栏消息中心 + 用户区
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useLocation, Outlet, Link } from 'react-router-dom';
import { ProLayout } from '@ant-design/pro-components';
import {
  DashboardOutlined,
  AppstoreOutlined,
  ShoppingOutlined,
  DatabaseOutlined,
  AccountBookOutlined,
  RobotOutlined,
  DeploymentUnitOutlined,
  CloudOutlined,
  BookOutlined,
  LogoutOutlined,
  UserOutlined,
  BellOutlined,
  SettingOutlined,
  CheckOutlined,
  WarningOutlined,
  ShoppingCartOutlined,
  AuditOutlined,
  InfoCircleOutlined,
  MessageOutlined,
} from '@ant-design/icons';
import {
  Dropdown,
  Space,
  Tag,
  Badge,
  Typography,
  Popover,
  List,
  Button,
  Empty,
  Spin,
  Divider,
} from 'antd';
import type { MenuDataItem } from '@ant-design/pro-components';
import { useAuthStore } from '@/store/auth';
import { useAppStore } from '@/store/app';
import { getUnreadCount, listMessages, markAllMessagesRead, markMessageRead } from '@/api/messages';
import { formatRelativeTime } from '@/utils/format';
import { ROLE_LABEL_MAP } from '@/utils/constants';
import type { MessageItem } from '@/types/api';

const { Text } = Typography;

const menuRouteMap: Record<string, string> = {
  '/dashboard': '工作台',
  '/products': '选品管理',
  '/purchases': '采购管理',
  '/messages': '客服消息',
  '/inventory': '库存管理',
  '/finance': '对账利润',
  '/ai-workflows': 'AI 工作流',
  '/ai-workflow-editor': '工作流编排',
  '/ai/knowledge-bases': '知识库',
  '/platforms': '平台对接',
};

const MSG_ICON: Record<string, React.ReactNode> = {
  stock_alert: <WarningOutlined style={{ color: '#cf1322' }} />,
  purchase: <ShoppingCartOutlined style={{ color: '#fa8c16' }} />,
  finance: <AuditOutlined style={{ color: '#13c2c2' }} />,
  system: <InfoCircleOutlined style={{ color: '#1677ff' }} />,
  ai: <RobotOutlined style={{ color: '#722ed1' }} />,
};

const BasicLayout: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, hasRole } = useAuthStore();
  const { collapsed, setCollapsed } = useAppStore();
  const [unread, setUnread] = useState(0);
  const [msgOpen, setMsgOpen] = useState(false);
  const [msgLoading, setMsgLoading] = useState(false);
  const [messages, setMessages] = useState<MessageItem[]>([]);

  const menuData = useMemo<MenuDataItem[]>(() => {
    const all: Array<MenuDataItem & { roles?: string[] }> = [
      { path: '/dashboard', name: '工作台', icon: <DashboardOutlined /> },
      { path: '/products', name: '选品管理', icon: <AppstoreOutlined /> },
      { path: '/purchases', name: '采购管理', icon: <ShoppingOutlined /> },
      { path: '/messages', name: '客服消息', icon: <MessageOutlined /> },
      { path: '/inventory', name: '库存管理', icon: <DatabaseOutlined /> },
      { path: '/finance', name: '对账利润', icon: <AccountBookOutlined />, roles: ['admin', 'manager', 'staff'] },
      { path: '/ai-workflows', name: 'AI 工作流', icon: <RobotOutlined /> },
      { path: '/ai-workflow-editor', name: '工作流编排', icon: <DeploymentUnitOutlined /> },
      { path: '/ai/knowledge-bases', name: '知识库', icon: <BookOutlined /> },
      { path: '/platforms', name: '平台对接', icon: <CloudOutlined />, roles: ['admin', 'manager'] },
    ];
    return all.filter((item) => {
      if (!item.roles?.length) return true;
      return hasRole(...item.roles);
    });
  }, [hasRole]);

  const refreshUnread = useCallback(async () => {
    try {
      const res = await getUnreadCount();
      setUnread(Number(res?.count || 0));
    } catch {
      // 静默
    }
  }, []);

  const loadMessages = useCallback(async () => {
    setMsgLoading(true);
    try {
      const res = await listMessages({ page: 1, page_size: 12 });
      setMessages(res?.list || []);
      await refreshUnread();
    } finally {
      setMsgLoading(false);
    }
  }, [refreshUnread]);

  useEffect(() => {
    refreshUnread();
    const timer = window.setInterval(refreshUnread, 60000);
    return () => window.clearInterval(timer);
  }, [refreshUnread]);

  const handleLogout = async () => {
    await logout();
    navigate('/login', { replace: true });
  };

  const handleOpenMsg = async (open: boolean) => {
    setMsgOpen(open);
    if (open) await loadMessages();
  };

  const handleRead = async (item: MessageItem) => {
    if (!item.is_read) {
      try {
        await markMessageRead(item.id);
        setMessages((prev) => prev.map((m) => (m.id === item.id ? { ...m, is_read: true } : m)));
        setUnread((n) => Math.max(0, n - 1));
      } catch {
        // ignore
      }
    }
    if (item.link) {
      setMsgOpen(false);
      navigate(item.link);
    }
  };

  const handleReadAll = async () => {
    try {
      await markAllMessagesRead();
      setMessages((prev) => prev.map((m) => ({ ...m, is_read: true })));
      setUnread(0);
    } catch {
      // ignore
    }
  };

  const matchedKey = Object.keys(menuRouteMap)
    .sort((a, b) => b.length - a.length)
    .find((k) => location.pathname === k || location.pathname.startsWith(`${k}/`));
  const pageTitle = matchedKey ? menuRouteMap[matchedKey] : 'CB-Platform';

  const roleLabel =
    ROLE_LABEL_MAP[user?.role || user?.roles?.[0] || ''] ||
    user?.department ||
    '企业工作区';

  const msgContent = (
    <div style={{ width: 360 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Text strong>消息中心</Text>
        <Button type="link" size="small" icon={<CheckOutlined />} onClick={handleReadAll} disabled={unread === 0}>
          全部已读
        </Button>
      </div>
      <Divider style={{ margin: '8px 0' }} />
      <Spin spinning={msgLoading}>
        {messages.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无消息" style={{ padding: '24px 0' }} />
        ) : (
          <List
            size="small"
            dataSource={messages}
            style={{ maxHeight: 380, overflowY: 'auto' }}
            renderItem={(item) => (
              <List.Item
                key={item.id}
                style={{
                  cursor: 'pointer',
                  background: item.is_read ? 'transparent' : '#f0f7ff',
                  borderRadius: 8,
                  padding: '10px 8px',
                  marginBottom: 4,
                }}
                onClick={() => handleRead(item)}
              >
                <List.Item.Meta
                  avatar={MSG_ICON[item.type] || MSG_ICON.system}
                  title={
                    <Space size={6}>
                      {!item.is_read && <Badge status="processing" />}
                      <Text strong={!item.is_read} style={{ fontSize: 13 }}>
                        {item.title}
                      </Text>
                    </Space>
                  }
                  description={
                    <div>
                      <div style={{ color: 'rgba(0,0,0,0.55)', fontSize: 12, lineHeight: 1.5 }}>
                        {item.content?.slice(0, 72)}
                        {item.content && item.content.length > 72 ? '…' : ''}
                      </div>
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        {formatRelativeTime(item.created_at)}
                      </Text>
                    </div>
                  }
                />
              </List.Item>
            )}
          />
        )}
      </Spin>
      <Divider style={{ margin: '8px 0' }} />
      <div style={{ textAlign: 'center' }}>
        <Button
          type="link"
          size="small"
          onClick={() => {
            setMsgOpen(false);
            navigate('/inventory');
          }}
        >
          查看库存预警
        </Button>
        <Button
          type="link"
          size="small"
          onClick={() => {
            setMsgOpen(false);
            navigate('/finance');
          }}
        >
          查看对账
        </Button>
      </div>
    </div>
  );

  return (
    <ProLayout
      title="CB-Platform"
      logo="/logo.svg"
      layout="side"
      fixedHeader
      fixSiderbar
      collapsed={collapsed}
      onCollapse={setCollapsed}
      navTheme="light"
      colorPrimary="#1677ff"
      contentWidth="Fluid"
      siderWidth={232}
      token={{
        header: {
          colorBgHeader: '#ffffff',
          colorHeaderTitle: 'rgba(0,0,0,0.88)',
          colorTextMenu: 'rgba(0,0,0,0.65)',
        },
        sider: {
          colorMenuBackground: '#ffffff',
          colorBgMenuItemSelected: '#e6f4ff',
          colorTextMenuSelected: '#1677ff',
          colorTextMenuItemHover: '#1677ff',
        },
        pageContainer: {
          paddingBlockPageContainerContent: 16,
          paddingInlinePageContainerContent: 20,
        },
      }}
      menu={{ request: async () => menuData, locale: false }}
      menuItemRender={(item, dom) => <Link to={item.path || '/'}>{dom}</Link>}
      avatarProps={{
        src: undefined,
        size: 'small',
        title: user?.nickname || '管理员',
        render: (_, dom) => (
          <Dropdown
            menu={{
              items: [
                {
                  key: 'profile',
                  icon: <UserOutlined />,
                  label: `${user?.nickname || user?.username || '用户'} · ${roleLabel}`,
                },
                {
                  key: 'settings',
                  icon: <SettingOutlined />,
                  label: '系统偏好',
                },
                { type: 'divider' },
                {
                  key: 'logout',
                  icon: <LogoutOutlined />,
                  label: '退出登录',
                  onClick: handleLogout,
                },
              ],
            }}
          >
            {dom}
          </Dropdown>
        ),
      }}
      actionsRender={() => [
        <Popover
          key="messages"
          content={msgContent}
          trigger="click"
          open={msgOpen}
          onOpenChange={handleOpenMsg}
          placement="bottomRight"
          arrow={{ pointAtCenter: true }}
        >
          <Badge count={unread} size="small" offset={[-2, 4]} overflowCount={99}>
            <BellOutlined
              style={{ fontSize: 16, color: 'rgba(0,0,0,0.65)', cursor: 'pointer', padding: 4 }}
            />
          </Badge>
        </Popover>,
      ]}
      footerRender={() => (
        <div style={{ textAlign: 'center', color: 'rgba(0,0,0,0.45)', fontSize: 12, padding: '12px 0' }}>
          CB-Platform · 选品 · 采购 · 库存 · 对账
        </div>
      )}
    >
      <div className="cbp-page-context">
        <Space size={10} wrap>
          <Tag color="processing" style={{ borderRadius: 6, margin: 0, fontWeight: 600 }}>
            {pageTitle}
          </Tag>
          {hasRole('admin') && (
            <Tag style={{ borderRadius: 6, margin: 0 }} color="purple">
              Admin
            </Tag>
          )}
        </Space>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {user?.nickname || '管理员'} · {roleLabel}
        </Text>
      </div>
      <Outlet />
    </ProLayout>
  );
};

export default BasicLayout;
