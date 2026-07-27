/**
 * 应用入口:挂载 React、Antd ConfigProvider(主题 token + zh_CN)
 */
import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider, App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import 'dayjs/locale/zh-cn';
import dayjs from 'dayjs';
import App from './App';
import './styles/global.css';

dayjs.locale('zh-cn');

const themeConfig = {
  token: {
    colorPrimary: '#2F54EB',
    colorInfo: '#2F54EB',
    colorLink: '#2F54EB',
    borderRadius: 6,
    fontFamily:
      "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    colorBgLayout: '#f0f2f5',
    siderBg: '#001529',
    fontSize: 14,
  },
  components: {
    Layout: {
      siderBg: '#001529',
      headerBg: '#ffffff',
      bodyBg: '#f0f2f5',
      headerHeight: 56,
    },
    Menu: {
      darkItemBg: '#001529',
      darkSubMenuItemBg: '#000c17',
      darkItemSelectedBg: '#2F54EB',
    },
    Card: {
      borderRadiusLG: 10,
    },
    Table: {
      headerBg: '#fafafa',
      headerColor: 'rgba(0, 0, 0, 0.88)',
      rowHoverBg: '#f5f7ff',
    },
    Button: {
      primaryShadow: '0 2px 4px rgba(47, 84, 235, 0.18)',
    },
  },
};

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN} theme={themeConfig}>
      <AntdApp>
        <App />
      </AntdApp>
    </ConfigProvider>
  </React.StrictMode>,
);
