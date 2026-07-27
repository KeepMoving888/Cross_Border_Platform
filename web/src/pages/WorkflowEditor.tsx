/**
 * 工作流可视化编排:基于 React Flow 的拖拽式 DAG 编排画布
 * - 左侧节点面板:拖拽生成节点(input / llm / rag / text2sql / sql_execute / condition / output)
 * - 中间画布:节点拖拽、连线、缩放
 * - 右侧属性面板:编辑节点名称与配置参数
 * - 顶部工具栏:保存 / 加载 / 清空 / 执行测试
 * 数据持久化到 localStorage(key: workflow_editor_data)
 */
import React, { useCallback, useMemo, useState } from 'react';
import {
  Button,
  Space,
  Typography,
  Input,
  Tag,
  Divider,
  Modal,
  Empty,
  Select,
  Tooltip,
  message,
} from 'antd';
import {
  PlayCircleOutlined,
  RobotOutlined,
  FileSearchOutlined,
  DatabaseOutlined,
  ConsoleSqlOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  SaveOutlined,
  FolderOpenOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  ExclamationCircleOutlined,
} from '@ant-design/icons';
import ReactFlow, {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  useReactFlow,
  ReactFlowProvider,
  Handle,
  Position,
  MarkerType,
} from 'reactflow';
import type { Node, Edge, Connection, NodeProps } from 'reactflow';
import 'reactflow/dist/style.css';
import PageContainer from '@/components/PageContainer';

const { Text, Title, Paragraph } = Typography;
const { TextArea } = Input;

/** 节点类型 key */
type NodeTypeKey = 'input' | 'llm' | 'rag' | 'text2sql' | 'sql_execute' | 'condition' | 'output';

/** 节点配置字段定义 */
interface ConfigField {
  key: string;
  label: string;
  type: 'text' | 'textarea' | 'select';
  placeholder?: string;
  options?: string[];
  defaultValue?: string;
}

/** 节点类型元数据 */
interface NodeTypeMeta {
  type: NodeTypeKey;
  label: string;
  color: string;
  icon: React.ReactNode;
  defaultName: string;
  hasInput: boolean;
  outputCount: 0 | 1 | 2;
  configFields: ConfigField[];
}

/** 工作流节点 data */
interface WorkflowNodeData {
  label: string;
  nodeType: NodeTypeKey;
  config: Record<string, string>;
}

/** localStorage 存储键 */
const STORAGE_KEY = 'workflow_editor_data';

/** 节点类型元数据表(颜色 / 图标 / 默认配置) */
const NODE_TYPE_META: Record<NodeTypeKey, NodeTypeMeta> = {
  input: {
    type: 'input',
    label: '输入节点',
    color: '#1677ff',
    icon: <PlayCircleOutlined />,
    defaultName: '用户输入',
    hasInput: false,
    outputCount: 1,
    configFields: [
      { key: 'input_var', label: '输入变量', type: 'text', placeholder: '如:user_query', defaultValue: 'user_query' },
    ],
  },
  llm: {
    type: 'llm',
    label: 'LLM 节点',
    color: '#722ed1',
    icon: <RobotOutlined />,
    defaultName: 'LLM 推理',
    hasInput: true,
    outputCount: 1,
    configFields: [
      {
        key: 'model',
        label: '模型',
        type: 'select',
        options: ['glm-4', 'glm-4-flash', 'claude-3-5-sonnet', 'deepseek-chat', 'qwen-plus'],
        defaultValue: 'glm-4',
      },
      { key: 'prompt', label: 'Prompt', type: 'textarea', placeholder: '请输入系统提示词...', defaultValue: '你是一个专业的助手。' },
      { key: 'temperature', label: 'Temperature', type: 'text', placeholder: '0.0 - 1.0', defaultValue: '0.7' },
    ],
  },
  rag: {
    type: 'rag',
    label: 'RAG 节点',
    color: '#13c2c2',
    icon: <FileSearchOutlined />,
    defaultName: '知识检索',
    hasInput: true,
    outputCount: 1,
    configFields: [
      { key: 'knowledge_base', label: '知识库', type: 'text', placeholder: '如:product_kb', defaultValue: 'product_kb' },
      { key: 'top_k', label: 'Top K', type: 'text', placeholder: '如:5', defaultValue: '5' },
    ],
  },
  text2sql: {
    type: 'text2sql',
    label: 'Text2SQL 节点',
    color: '#fa8c16',
    icon: <DatabaseOutlined />,
    defaultName: 'Text2SQL',
    hasInput: true,
    outputCount: 1,
    configFields: [
      { key: 'datasource', label: '数据源', type: 'text', placeholder: '如:cb_platform', defaultValue: 'cb_platform' },
      { key: 'prompt', label: 'Prompt', type: 'textarea', placeholder: '请输入 NL2SQL 提示词...', defaultValue: '将用户问题翻译为可执行的 SQL。' },
    ],
  },
  sql_execute: {
    type: 'sql_execute',
    label: 'SQL 执行节点',
    color: '#52c41a',
    icon: <ConsoleSqlOutlined />,
    defaultName: 'SQL 执行',
    hasInput: true,
    outputCount: 1,
    configFields: [
      { key: 'datasource', label: '数据源', type: 'text', placeholder: '如:cb_platform', defaultValue: 'cb_platform' },
      { key: 'sql', label: 'SQL', type: 'textarea', placeholder: 'SELECT ...', defaultValue: 'SELECT 1;' },
    ],
  },
  condition: {
    type: 'condition',
    label: '条件节点',
    color: '#faad14',
    icon: <BranchesOutlined />,
    defaultName: '条件分支',
    hasInput: true,
    outputCount: 2,
    configFields: [
      { key: 'expression', label: '条件表达式', type: 'textarea', placeholder: '如:score >= 80', defaultValue: 'score >= 80' },
    ],
  },
  output: {
    type: 'output',
    label: '输出节点',
    color: '#52c41a',
    icon: <CheckCircleOutlined />,
    defaultName: '结果输出',
    hasInput: true,
    outputCount: 0,
    configFields: [
      { key: 'format', label: '输出格式', type: 'select', options: ['text', 'json', 'markdown'], defaultValue: 'text' },
      { key: 'template', label: '输出模板', type: 'textarea', placeholder: '{{result}}', defaultValue: '{{result}}' },
    ],
  },
};

/** 侧栏可拖拽节点顺序 */
const SIDEBAR_NODE_ORDER: NodeTypeKey[] = [
  'input',
  'llm',
  'rag',
  'text2sql',
  'sql_execute',
  'condition',
  'output',
];

/** 自定义节点渲染组件 */
const WorkflowNode: React.FC<NodeProps<WorkflowNodeData>> = ({ data, selected }) => {
  const meta = NODE_TYPE_META[data.nodeType];
  const color = meta.color;
  return (
    <div
      style={{
        width: 180,
        borderRadius: 10,
        border: `2px solid ${selected ? color : 'transparent'}`,
        boxShadow: selected ? `0 0 0 2px ${color}33` : '0 2px 8px rgba(0,0,0,0.1)',
        background: '#fff',
        overflow: 'visible',
      }}
    >
      {/* 顶部:节点类型图标 + 名称 */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '8px 12px',
          background: `${color}1a`,
          color,
          borderRadius: '8px 8px 0 0',
        }}
      >
        <span style={{ fontSize: 16, display: 'flex', alignItems: 'center' }}>{meta.icon}</span>
        <Text strong style={{ flex: 1, color, fontSize: 13 }} ellipsis>
          {data.label || meta.defaultName}
        </Text>
      </div>
      {/* 底部:类型标签 */}
      <div style={{ padding: '6px 12px' }}>
        <Tag style={{ margin: 0, fontSize: 11, color, borderColor: color, background: `${color}1a` }}>
          {meta.label}
        </Tag>
      </div>

      {/* 输入连接点 */}
      {meta.hasInput && (
        <Handle
          type="target"
          position={Position.Top}
          style={{ background: color, width: 10, height: 10, border: '2px solid #fff' }}
        />
      )}
      {/* 输出连接点:单输出在底部居中,双输出(条件)在底部 30% / 70% */}
      {meta.outputCount === 1 && (
        <Handle
          type="source"
          position={Position.Bottom}
          style={{ background: color, width: 10, height: 10, border: '2px solid #fff' }}
        />
      )}
      {meta.outputCount === 2 && (
        <>
          <Tooltip title="真 (true)">
            <Handle
              id="true"
              type="source"
              position={Position.Bottom}
              style={{ left: '30%', background: '#52c41a', width: 10, height: 10, border: '2px solid #fff' }}
            />
          </Tooltip>
          <Tooltip title="假 (false)">
            <Handle
              id="false"
              type="source"
              position={Position.Bottom}
              style={{ left: '70%', background: '#ff4d4f', width: 10, height: 10, border: '2px solid #fff' }}
            />
          </Tooltip>
        </>
      )}
    </div>
  );
};

/** nodeTypes 必须在组件外定义,避免每次渲染重建导致 React Flow 警告 */
const nodeTypes = { workflow: WorkflowNode };

/** 构造初始示例工作流:input → llm → output */
const buildInitialNodes = (): Node<WorkflowNodeData>[] => [
  {
    id: 'input-1',
    type: 'workflow',
    position: { x: 320, y: 40 },
    data: { label: '用户输入', nodeType: 'input', config: { input_var: 'user_query' } },
  },
  {
    id: 'llm-1',
    type: 'workflow',
    position: { x: 320, y: 220 },
    data: {
      label: 'LLM 推理',
      nodeType: 'llm',
      config: { model: 'glm-4', prompt: '你是一个专业的助手。', temperature: '0.7' },
    },
  },
  {
    id: 'output-1',
    type: 'workflow',
    position: { x: 320, y: 400 },
    data: { label: '结果输出', nodeType: 'output', config: { format: 'text', template: '{{result}}' } },
  },
];

/** 构造初始示例连线 */
const buildInitialEdges = (): Edge[] => [
  {
    id: 'e-input-llm',
    source: 'input-1',
    target: 'llm-1',
    animated: true,
    markerEnd: { type: MarkerType.ArrowClosed, color: '#b1b1b7' },
  },
  {
    id: 'e-llm-output',
    source: 'llm-1',
    target: 'output-1',
    animated: true,
    markerEnd: { type: MarkerType.ArrowClosed, color: '#b1b1b7' },
  },
];

/** 从 localStorage 读取已保存的工作流 */
const loadFromStorage = (): { nodes: Node<WorkflowNodeData>[]; edges: Edge[] } | null => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed.nodes) || !Array.isArray(parsed.edges)) return null;
    return {
      nodes: parsed.nodes as Node<WorkflowNodeData>[],
      edges: parsed.edges as Edge[],
    };
  } catch {
    return null;
  }
};

/** 序列化当前画布为可存储的 JSON(剔除内部运行时字段) */
const serializeFlow = (nodes: Node<WorkflowNodeData>[], edges: Edge[]) => ({
  nodes: nodes.map((n) => ({
    id: n.id,
    type: n.type,
    position: n.position,
    data: n.data,
  })),
  edges: edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    sourceHandle: e.sourceHandle,
    targetHandle: e.targetHandle,
    animated: e.animated,
  })),
});

/** 编辑器内部组件(需在 ReactFlowProvider 内以使用 useReactFlow) */
const EditorInner: React.FC = () => {
  const { screenToFlowPosition } = useReactFlow<WorkflowNodeData>();

  // 初始化:优先从 localStorage 加载,否则使用示例工作流
  const initialData = useMemo(() => {
    const stored = loadFromStorage();
    if (stored && stored.nodes.length > 0) return stored;
    return { nodes: buildInitialNodes(), edges: buildInitialEdges() };
  }, []);

  const [nodes, setNodes, onNodesChange] = useNodesState<WorkflowNodeData>(initialData.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialData.edges);
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [testResult, setTestResult] = useState('');

  /** 当前选中节点 */
  const selectedNode = useMemo(() => nodes.find((n) => n.selected) ?? null, [nodes]);

  /** 从侧栏开始拖拽 */
  const onDragStart = (event: React.DragEvent, nodeType: NodeTypeKey) => {
    event.dataTransfer.setData('application/reactflow', nodeType);
    event.dataTransfer.effectAllowed = 'move';
  };

  /** 拖拽悬停:阻止默认行为以允许 drop */
  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  /** 拖拽释放:在画布对应位置生成新节点 */
  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const type = event.dataTransfer.getData('application/reactflow') as NodeTypeKey;
      if (!type || !NODE_TYPE_META[type]) return;
      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const meta = NODE_TYPE_META[type];
      // 填充默认配置
      const config: Record<string, string> = {};
      meta.configFields.forEach((f) => {
        if (f.defaultValue !== undefined) config[f.key] = f.defaultValue;
      });
      const newNode: Node<WorkflowNodeData> = {
        id: `${type}-${Date.now()}`,
        type: 'workflow',
        position,
        data: { label: meta.defaultName, nodeType: type, config },
      };
      setNodes((nds) => nds.concat(newNode));
    },
    [screenToFlowPosition, setNodes],
  );

  /** 连线校验:每个节点最多 1 个输入、最多 2 个输出 */
  const isValidConnection = useCallback(
    (connection: Edge | Connection) => {
      const targetId = connection.target;
      const sourceId = connection.source;
      // 最多 1 个输入
      if (targetId && edges.some((e) => e.target === targetId)) return false;
      // 最多 2 个输出
      if (sourceId && edges.filter((e) => e.source === sourceId).length >= 2) return false;
      return true;
    },
    [edges],
  );

  /** 建立连线 */
  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) =>
        addEdge(
          { ...connection, animated: true, markerEnd: { type: MarkerType.ArrowClosed, color: '#b1b1b7' } },
          eds,
        ),
      );
    },
    [setEdges],
  );

  /** 更新节点名称 */
  const updateNodeLabel = useCallback(
    (id: string, label: string) => {
      setNodes((nds) => nds.map((n) => (n.id === id ? { ...n, data: { ...n.data, label } } : n)));
    },
    [setNodes],
  );

  /** 更新节点配置项 */
  const updateNodeConfig = useCallback(
    (id: string, key: string, value: string) => {
      setNodes((nds) =>
        nds.map((n) => (n.id === id ? { ...n, data: { ...n.data, config: { ...n.data.config, [key]: value } } } : n)),
      );
    },
    [setNodes],
  );

  /** 删除节点及其关联连线 */
  const deleteNode = useCallback(
    (id: string) => {
      setNodes((nds) => nds.filter((n) => n.id !== id));
      setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id));
    },
    [setNodes, setEdges],
  );

  /** 保存工作流到 localStorage */
  const handleSave = () => {
    const payload = serializeFlow(nodes, edges);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
    message.success(`已保存工作流(${payload.nodes.length} 节点,${payload.edges.length} 连线)`);
  };

  /** 从 localStorage 加载工作流 */
  const handleLoad = () => {
    const data = loadFromStorage();
    if (!data || data.nodes.length === 0) {
      message.warning('暂无可加载的工作流数据');
      return;
    }
    setNodes(data.nodes);
    setEdges(data.edges);
    message.success(`已加载工作流(${data.nodes.length} 节点,${data.edges.length} 连线)`);
  };

  /** 清空画布 */
  const handleClear = () => {
    Modal.confirm({
      title: '确认清空画布?',
      icon: <ExclamationCircleOutlined />,
      content: '将移除画布上的所有节点与连线,未保存的数据将丢失。',
      okText: '清空',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: () => {
        setNodes([]);
        setEdges([]);
        message.success('画布已清空');
      },
    });
  };

  /** 执行测试:序列化为 JSON 并展示(Demo,不执行真实引擎) */
  const handleExecute = () => {
    if (nodes.length === 0) {
      message.warning('画布为空,无法执行');
      return;
    }
    const payload = serializeFlow(nodes, edges);
    setTestResult(JSON.stringify(payload, null, 2));
    setTestModalOpen(true);
  };

  return (
    <PageContainer title="工作流可视化编排" breadcrumb={{}} subTitle="">
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          height: 'calc(100vh - 200px)',
          minHeight: 500,
          background: '#fff',
          borderRadius: 10,
          border: '1px solid #f0f0f0',
          overflow: 'hidden',
        }}
      >
        {/* 顶部工具栏 */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '10px 16px',
            borderBottom: '1px solid #f0f0f0',
            background: '#fafafa',
          }}
        >
          <Space size={6} wrap>
            <Text strong style={{ fontSize: 14 }}>工作流编排</Text>
            <Tag color="blue">{nodes.length} 节点</Tag>
            <Tag color="default">{edges.length} 连线</Tag>
          </Space>
          <Space size={8} wrap>
            <Button icon={<SaveOutlined />} onClick={handleSave}>
              保存工作流
            </Button>
            <Button icon={<FolderOpenOutlined />} onClick={handleLoad}>
              加载工作流
            </Button>
            <Button danger icon={<DeleteOutlined />} onClick={handleClear}>
              清空画布
            </Button>
            <Button type="primary" icon={<ThunderboltOutlined />} onClick={handleExecute}>
              执行测试
            </Button>
          </Space>
        </div>

        {/* 主体:左侧面板 + 中间画布 + 右侧属性 */}
        <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
          {/* 左侧:节点面板 */}
          <div
            style={{
              width: 200,
              flexShrink: 0,
              borderRight: '1px solid #f0f0f0',
              padding: 12,
              overflowY: 'auto',
              background: '#fcfcfc',
            }}
          >
            <Text type="secondary" style={{ fontSize: 12 }}>
              节点类型(拖拽到画布)
            </Text>
            <div style={{ marginTop: 10 }}>
              {SIDEBAR_NODE_ORDER.map((key) => {
                const meta = NODE_TYPE_META[key];
                return (
                  <div
                    key={key}
                    draggable
                    onDragStart={(e) => onDragStart(e, key)}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '8px 10px',
                      borderRadius: 8,
                      border: '1px solid #f0f0f0',
                      background: '#fff',
                      cursor: 'grab',
                      marginBottom: 8,
                      transition: 'box-shadow 0.2s',
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.boxShadow = '0 2px 8px rgba(0,0,0,0.12)';
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.boxShadow = 'none';
                    }}
                  >
                    <span style={{ color: meta.color, fontSize: 16, display: 'flex', alignItems: 'center' }}>
                      {meta.icon}
                    </span>
                    <Text style={{ fontSize: 13 }}>{meta.label}</Text>
                  </div>
                );
              })}
            </div>
          </div>

          {/* 中间:React Flow 画布 */}
          <div
            style={{ flex: 1, minWidth: 0, position: 'relative' }}
            onDrop={onDrop}
            onDragOver={onDragOver}
          >
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              isValidConnection={isValidConnection}
              fitView
              minZoom={0.2}
              maxZoom={2}
              defaultEdgeOptions={{
                animated: true,
                markerEnd: { type: MarkerType.ArrowClosed, color: '#b1b1b7' },
              }}
              style={{ width: '100%', height: '100%', background: '#f7f9fc' }}
            >
              <Background variant={BackgroundVariant.Dots} gap={16} size={1} color="#d9d9d9" />
              <Controls showInteractive={false} />
              <MiniMap
                nodeStrokeColor={(n) => NODE_TYPE_META[(n.data as WorkflowNodeData)?.nodeType]?.color || '#b1b1b7'}
                nodeColor={(n) => NODE_TYPE_META[(n.data as WorkflowNodeData)?.nodeType]?.color || '#b1b1b7'}
                style={{ borderRadius: 8 }}
              />
            </ReactFlow>
          </div>

          {/* 右侧:节点属性面板 */}
          <div
            style={{
              width: 300,
              flexShrink: 0,
              borderLeft: '1px solid #f0f0f0',
              padding: 16,
              overflowY: 'auto',
              background: '#fcfcfc',
            }}
          >
            {selectedNode ? (
              (() => {
                const meta = NODE_TYPE_META[selectedNode.data.nodeType];
                return (
                  <div>
                    <Title level={5} style={{ marginTop: 0 }}>
                      节点属性
                    </Title>
                    {/* 节点名称 */}
                    <div style={{ marginBottom: 12 }}>
                      <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        节点名称
                      </Text>
                      <Input
                        value={selectedNode.data.label}
                        onChange={(e) => updateNodeLabel(selectedNode.id, e.target.value)}
                        placeholder="请输入节点名称"
                      />
                    </div>
                    {/* 节点类型(只读) */}
                    <div style={{ marginBottom: 12 }}>
                      <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                        节点类型
                      </Text>
                      <Tag style={{ color: meta.color, borderColor: meta.color, background: `${meta.color}1a` }}>
                        {meta.icon} {meta.label}
                      </Tag>
                    </div>
                    <Divider style={{ margin: '12px 0' }}>配置参数</Divider>
                    {/* 动态配置项 */}
                    {meta.configFields.map((field) => (
                      <div key={field.key} style={{ marginBottom: 12 }}>
                        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
                          {field.label}
                        </Text>
                        {field.type === 'textarea' ? (
                          <TextArea
                            rows={3}
                            value={selectedNode.data.config[field.key] ?? ''}
                            onChange={(e) => updateNodeConfig(selectedNode.id, field.key, e.target.value)}
                            placeholder={field.placeholder}
                          />
                        ) : field.type === 'select' ? (
                          <Select
                            value={selectedNode.data.config[field.key] ?? field.defaultValue}
                            onChange={(v) => updateNodeConfig(selectedNode.id, field.key, v as string)}
                            options={field.options?.map((o) => ({ value: o, label: o }))}
                            style={{ width: '100%' }}
                            placeholder={field.placeholder}
                          />
                        ) : (
                          <Input
                            value={selectedNode.data.config[field.key] ?? ''}
                            onChange={(e) => updateNodeConfig(selectedNode.id, field.key, e.target.value)}
                            placeholder={field.placeholder}
                          />
                        )}
                      </div>
                    ))}
                    <Button
                      danger
                      icon={<DeleteOutlined />}
                      block
                      onClick={() => deleteNode(selectedNode.id)}
                      style={{ marginTop: 8 }}
                    >
                      删除该节点
                    </Button>
                  </div>
                );
              })()
            ) : (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="点击画布节点查看并编辑属性"
                style={{ marginTop: 60 }}
              />
            )}
          </div>
        </div>
      </div>

      {/* 执行测试结果弹窗(Demo:展示序列化 JSON) */}
      <Modal
        title={
          <Space>
            <ThunderboltOutlined style={{ color: '#1677ff' }} />
            <span>执行测试 · 工作流 JSON 预览</span>
          </Space>
        }
        open={testModalOpen}
        onCancel={() => setTestModalOpen(false)}
        footer={[
          <Button
            key="copy"
            onClick={() => {
              navigator.clipboard
                .writeText(testResult)
                .then(() => message.success('已复制 JSON 到剪贴板'))
                .catch(() => message.error('复制失败'));
            }}
          >
            复制 JSON
          </Button>,
          <Button key="close" type="primary" onClick={() => setTestModalOpen(false)}>
            关闭
          </Button>,
        ]}
        width={720}
      >
        <Paragraph type="secondary" style={{ marginBottom: 12 }}>
          当前为 Demo 模式,仅展示工作流序列化结果。实际执行需后端按节点拓扑执行(input → LLM/RAG/Text2SQL → output)。
        </Paragraph>
        <pre
          style={{
            background: '#0f172a',
            color: '#e2e8f0',
            padding: 16,
            borderRadius: 8,
            maxHeight: 420,
            overflow: 'auto',
            fontSize: 12,
            lineHeight: 1.6,
          }}
        >
          {testResult}
        </pre>
      </Modal>
    </PageContainer>
  );
};

/** 工作流编排页面(外层包裹 ReactFlowProvider) */
const WorkflowEditor: React.FC = () => (
  <ReactFlowProvider>
    <EditorInner />
  </ReactFlowProvider>
);

export default WorkflowEditor;
