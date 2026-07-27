/**
 * 登录页:居中卡片式,品牌 Logo + 标语
 */
import React, { useState } from 'react';
import { Card, Form, Input, Button, Typography, Alert, Space } from 'antd';
import { UserOutlined, LockOutlined, ShopOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '@/store/auth';

const { Title, Text } = Typography;

const Login: React.FC = () => {
  const navigate = useNavigate();
  const { login, loading } = useAuthStore();
  const [errorMsg, setErrorMsg] = useState<string>('');

  const onFinish = async (values: { username: string; password: string }) => {
    setErrorMsg('');
    try {
      await login(values);
      navigate('/dashboard', { replace: true });
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : '登录失败,请检查账号密码');
    }
  };

  return (
    <div className="login-page">
      <Card
        style={{
          width: 420,
          borderRadius: 12,
          boxShadow: '0 16px 48px rgba(0, 0, 0, 0.18)',
          border: 'none',
        }}
        styles={{ body: { padding: '36px 36px 28px' } }}
      >
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <Space direction="vertical" size={6} style={{ width: '100%' }}>
            <div
              style={{
                width: 56,
                height: 56,
                margin: '0 auto 8px',
                borderRadius: 14,
                background: 'linear-gradient(135deg, #2F54EB 0%, #1677ff 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#fff',
                fontSize: 26,
              }}
            >
              <ShopOutlined />
            </div>
            <Title level={4} style={{ margin: 0, fontWeight: 600 }}>
              CB-Platform
            </Title>
            <Text type="secondary">跨境电商智能运营中台</Text>
          </Space>
        </div>

        {errorMsg && (
          <Alert
            type="error"
            message={errorMsg}
            showIcon
            style={{ marginBottom: 16 }}
            closable
            onClose={() => setErrorMsg('')}
          />
        )}

        <Form
          name="login"
          initialValues={{ username: 'admin', password: 'admin123' }}
          onFinish={onFinish}
          size="large"
          autoComplete="off"
        >
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="current-password" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 12 }}>
            <Button
              type="primary"
              htmlType="submit"
              block
              loading={loading}
              style={{
                height: 44,
                background: 'linear-gradient(90deg, #2F54EB 0%, #1677ff 100%)',
                border: 'none',
                fontWeight: 500,
              }}
            >
              登 录
            </Button>
          </Form.Item>
        </Form>

        <div style={{ textAlign: 'center', color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
          默认账号:admin / admin123 · 仅限内部运营人员使用
        </div>
      </Card>
      <div
        style={{
          position: 'absolute',
          bottom: 24,
          color: 'rgba(255,255,255,0.7)',
          fontSize: 12,
        }}
      >
        © 2026 CB-Platform · 家电美容跨境运营中台
      </div>
    </div>
  );
};

export default Login;
