/**
 * 路由配置 + 路由守卫
 */
import React from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import BasicLayout from '@/layouts/BasicLayout';
import Login from '@/pages/Login';
import Dashboard from '@/pages/Dashboard';
import Products from '@/pages/Products';
import ProductDetail from '@/pages/ProductDetail';
import Purchases from '@/pages/Purchases';
import Inventory from '@/pages/Inventory';
import Finance from '@/pages/Finance';
import AIWorkflows from '@/pages/AIWorkflows';
import AIWorkflowRuns from '@/pages/AIWorkflowRuns';
import KnowledgeBases from '@/pages/KnowledgeBases';
import WorkflowEditor from '@/pages/WorkflowEditor';
import Platforms from '@/pages/Platforms';
import Messages from '@/pages/Messages';
import { useAuthStore } from '@/store/auth';

/** 路由守卫:未登录跳 /login */
const RequireAuth: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const location = useLocation();
  const token = useAuthStore((s) => s.token);
  if (!token) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }
  return <>{children}</>;
};

/** 已登录用户访问 /login 自动跳工作台 */
const RedirectIfAuthed: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = useAuthStore((s) => s.token);
  if (token) {
    return <Navigate to="/dashboard" replace />;
  }
  return <>{children}</>;
};

const App: React.FC = () => {
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={
            <RedirectIfAuthed>
              <Login />
            </RedirectIfAuthed>
          }
        />
        <Route
          path="/"
          element={
            <RequireAuth>
              <BasicLayout />
            </RequireAuth>
          }
        >
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="products" element={<Products />} />
          <Route path="products/:id" element={<ProductDetail />} />
          <Route path="purchases" element={<Purchases />} />
          <Route path="messages" element={<Messages />} />
          <Route path="inventory" element={<Inventory />} />
          <Route path="finance" element={<Finance />} />
          <Route path="ai-workflows" element={<AIWorkflows />} />
          <Route path="ai-workflow-editor" element={<WorkflowEditor />} />
          <Route path="/ai/runs" element={<AIWorkflowRuns />} />
          <Route path="/ai/knowledge-bases" element={<KnowledgeBases />} />
          <Route path="platforms" element={<Platforms />} />
        </Route>
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Routes>
    </BrowserRouter>
  );
};

export default App;
