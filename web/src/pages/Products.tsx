import React, { useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Button,
  Modal,
  Tag,
  Space,
  Tooltip,
  Typography,
  message,
  Form,
  Input,
  InputNumber,
  Select,
  Popconfirm,
  Dropdown,
} from 'antd';
import {
  RobotOutlined,
  StarFilled,
  EyeOutlined,
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import {
  listProducts,
  createProduct,
  updateProduct,
  changeProductStage,
  deleteProduct,
} from '@/api/products';
import { useAuthStore } from '@/store/auth';
import {
  PRODUCT_CATEGORY_MAP,
  PRODUCT_STAGE_MAP,
  PLATFORM_MAP,
  TARGET_MARKET_MAP,
  CATEGORY_OPTIONS,
  STAGE_OPTIONS,
  PLATFORM_OPTIONS,
} from '@/utils/constants';
import { formatUSD, formatPercent, formatNumber, formatRating } from '@/utils/format';
import type { Product, ProductQuery, ProductStage, TargetMarket } from '@/types/api';

const { Paragraph, Text } = Typography;

const MARKET_OPTIONS = Object.entries(TARGET_MARKET_MAP).map(([value, label]) => ({
  value: value as TargetMarket,
  label,
}));

const Products: React.FC = () => {
  const navigate = useNavigate();
  const actionRef = useRef<ActionType>(null);
  const hasRole = useAuthStore((s) => s.hasRole);
  const canManage = hasRole('admin', 'manager');
  const canDelete = hasRole('admin');

  const [insightModal, setInsightModal] = useState<{ open: boolean; product?: Product }>({
    open: false,
  });
  const [formModal, setFormModal] = useState<{
    open: boolean;
    mode: 'create' | 'edit';
    record?: Product;
  }>({ open: false, mode: 'create' });
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);

  const openCreate = () => {
    form.resetFields();
    form.setFieldsValue({
      stage: 'sourcing',
      platform: 'amazon',
      target_market: 'US',
      category: 'personal_care',
    });
    setFormModal({ open: true, mode: 'create' });
  };

  const openEdit = (record: Product) => {
    const raw = record.tags as unknown as string[] | string | undefined;
    const tagsStr = Array.isArray(raw)
      ? raw.join(',')
      : typeof raw === 'string'
        ? raw
        : '';
    form.setFieldsValue({
      sku: record.sku,
      name: record.name,
      category: record.category,
      stage: record.stage,
      platform: record.platform,
      target_market: record.target_market,
      list_price: record.list_price,
      est_cost_price: record.est_cost_price,
      monthly_sales: record.monthly_sales,
      tags: tagsStr,
      remark: '',
    });
    setFormModal({ open: true, mode: 'edit', record });
  };

  const handleSubmit = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const payload = {
        sku: values.sku,
        name: values.name,
        category: values.category,
        stage: values.stage,
        platform: values.platform,
        target_market: values.target_market,
        list_price: values.list_price,
        est_cost_price: values.est_cost_price,
        monthly_sales: values.monthly_sales,
        tags: values.tags || undefined,
        remark: values.remark || undefined,
      };
      if (formModal.mode === 'create') {
        await createProduct(payload);
        message.success('选品创建成功');
      } else if (formModal.record) {
        await updateProduct(formModal.record.id, payload);
        message.success('选品更新成功');
      }
      setFormModal({ open: false, mode: 'create' });
      form.resetFields();
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setSubmitting(false);
    }
  };

  const handleStageChange = async (record: Product, stage: ProductStage) => {
    if (stage === record.stage) return;
    try {
      await changeProductStage(record.id, { stage });
      message.success(`阶段已变更为「${PRODUCT_STAGE_MAP[stage]?.label || stage}」`);
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    }
  };

  const handleDelete = async (record: Product) => {
    try {
      await deleteProduct(record.id);
      message.success('选品已删除');
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    }
  };

  const columns: ProColumns<Product>[] = [
    {
      title: 'SKU / ASIN',
      dataIndex: 'sku',
      width: 180,
      copyable: true,
      render: (_, r) => (
        <Space direction="vertical" size={0}>
          <Text strong>{r.sku}</Text>
          {r.asin && <Text type="secondary" style={{ fontSize: 12 }}>{r.asin}</Text>}
        </Space>
      ),
    },
    {
      title: '商品名称',
      dataIndex: 'name',
      ellipsis: true,
      width: 240,
      search: false,
      render: (_, r) => (
        <a onClick={() => navigate(`/products/${r.id}`)}>{r.name}</a>
      ),
    },
    {
      title: '名称搜索',
      dataIndex: 'keyword',
      hideInTable: true,
    },
    {
      title: '类目',
      dataIndex: 'category',
      width: 110,
      valueType: 'select',
      fieldProps: { options: CATEGORY_OPTIONS },
      render: (_, r) => (
        <StatusTag status={r.category} map={PRODUCT_CATEGORY_MAP} />
      ),
    },
    {
      title: '阶段',
      dataIndex: 'stage',
      width: 100,
      valueType: 'select',
      fieldProps: { options: STAGE_OPTIONS },
      render: (_, r) => (
        <StatusTag status={r.stage} map={PRODUCT_STAGE_MAP} />
      ),
    },
    {
      title: '平台',
      dataIndex: 'platform',
      width: 100,
      valueType: 'select',
      fieldProps: { options: PLATFORM_OPTIONS },
      render: (_, r) => (
        <StatusTag status={r.platform} map={PLATFORM_MAP} />
      ),
    },
    {
      title: '目标市场',
      dataIndex: 'target_market',
      width: 90,
      search: false,
      render: (_, r) => TARGET_MARKET_MAP[r.target_market] || r.target_market,
    },
    {
      title: '上架价',
      dataIndex: 'list_price',
      width: 100,
      search: false,
      align: 'right',
      sorter: true,
      render: (_, r) => formatUSD(r.list_price),
    },
    {
      title: '预估毛利率',
      dataIndex: 'est_margin_rate',
      width: 110,
      search: false,
      align: 'right',
      sorter: true,
      render: (_, r) => {
        const rate = Number(r.est_margin_rate ?? 0) / 100;
        return (
          <span style={{ color: rate >= 0.6 ? '#52c41a' : 'inherit' }}>
            {formatPercent(rate)}
          </span>
        );
      },
    },
    {
      title: 'AI 评分',
      dataIndex: 'ai_score',
      width: 110,
      align: 'right',
      sorter: true,
      defaultSortOrder: 'descend',
      render: (_, r) => (
        <Space size={4}>
          <StarFilled style={{ color: '#faad14' }} />
          <Text strong style={{ color: r.ai_score >= 85 ? '#2F54EB' : 'inherit' }}>
            {r.ai_score}
          </Text>
        </Space>
      ),
    },
    {
      title: '月销量',
      dataIndex: 'monthly_sales',
      width: 100,
      search: false,
      align: 'right',
      sorter: true,
      render: (_, r) => formatNumber(r.monthly_sales),
    },
    {
      title: '评分',
      dataIndex: 'rating',
      width: 80,
      search: false,
      align: 'right',
      render: (_, r) => (Number(r.rating) > 0 ? formatRating(r.rating) : '-'),
    },
    {
      title: '标签',
      dataIndex: 'tags',
      width: 180,
      search: false,
      render: (_, r) => {
        const raw = r.tags as unknown as string[] | string | undefined;
        const tags: string[] = Array.isArray(raw)
          ? raw
          : typeof raw === 'string'
            ? raw.split(',').filter(Boolean)
            : [];
        return (
          <Space size={[0, 4]} wrap>
            {tags.map((t: string) => (
              <Tag key={t} style={{ borderRadius: 4 }}>
                {t}
              </Tag>
            ))}
          </Space>
        );
      },
    },
    {
      title: '操作',
      valueType: 'option',
      width: canManage ? 280 : 170,
      fixed: 'right',
      render: (_, r) => {
        const actions = [
          <Tooltip title="查看详情(趋势/竞品/AI 洞察)" key="detail">
            <Button
              type="link"
              size="small"
              icon={<EyeOutlined />}
              onClick={() => navigate(`/products/${r.id}`)}
            >
              详情
            </Button>
          </Tooltip>,
          <Tooltip title="查看 AI 洞察" key="ai">
            <Button
              type="link"
              size="small"
              icon={<RobotOutlined />}
              onClick={() => setInsightModal({ open: true, product: r })}
            >
              AI 分析
            </Button>
          </Tooltip>,
        ];

        if (canManage) {
          actions.push(
            <Dropdown
              key="stage"
              menu={{
                items: STAGE_OPTIONS.map((opt) => ({
                  key: opt.value,
                  label: opt.label,
                  disabled: opt.value === r.stage,
                })),
                onClick: ({ key }) => handleStageChange(r, key as ProductStage),
              }}
            >
              <Button type="link" size="small" icon={<SwapOutlined />}>
                变更阶段
              </Button>
            </Dropdown>,
            <Button
              key="edit"
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => openEdit(r)}
            >
              编辑
            </Button>,
          );
        }

        if (canDelete) {
          actions.push(
            <Popconfirm
              key="delete"
              title="确认删除该选品？"
              description="删除后不可恢复，请谨慎操作。"
              okText="确认删除"
              cancelText="取消"
              okButtonProps={{ danger: true }}
              onConfirm={() => handleDelete(r)}
            >
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>,
          );
        }

        return actions;
      },
    },
  ];

  return (
    <PageContainer title="选品管理" breadcrumb={{}}>
      <ProTable<Product>
        cardBordered
        rowKey="id"
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: canManage ? 1700 : 1600 }}
        search={{ labelWidth: 80, defaultCollapsed: false }}
        request={async (params) => {
          const query: ProductQuery = {
            page: params.current,
            page_size: params.pageSize,
            keyword: params.keyword,
            category: params.category as Product['category'],
            stage: params.stage as Product['stage'],
            platform: params.platform as Product['platform'],
          };
          for (const k of ['ai_score', 'list_price', 'est_margin_rate', 'monthly_sales']) {
            if (params[k as keyof typeof params]) {
              query.sort_by = k as ProductQuery['sort_by'];
              query.sort_order = (params as Record<string, string>).sortOrder === 'ascend' ? 'asc' : 'desc';
            }
          }
          const res = await listProducts(query);
          return {
            data: res.list,
            total: res.total,
            success: true,
          };
        }}
        pagination={{
          pageSize: 10,
          showSizeChanger: true,
        }}
        options={{ density: false, fullScreen: true, reload: true }}
        headerTitle="选品列表"
        toolBarRender={() =>
          canManage
            ? [
                <Button key="create" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                  新建选品
                </Button>,
              ]
            : []
        }
      />

      <Modal
        title={
          <Space>
            <RobotOutlined style={{ color: '#2F54EB' }} />
            <span>AI 选品洞察 · {insightModal.product?.name}</span>
          </Space>
        }
        open={insightModal.open}
        onCancel={() => setInsightModal({ open: false })}
        footer={[
          <Button key="close" type="primary" onClick={() => setInsightModal({ open: false })}>
            知道了
          </Button>,
        ]}
        width={640}
      >
        {insightModal.product && (
          <div style={{ marginBottom: 12 }}>
            <Space size={8} wrap>
              <Tag color="blue">{insightModal.product.sku}</Tag>
              <StatusTag status={insightModal.product.category} map={PRODUCT_CATEGORY_MAP} />
              <StatusTag status={insightModal.product.stage} map={PRODUCT_STAGE_MAP} />
              <Tag>AI 评分 {insightModal.product.ai_score}</Tag>
            </Space>
          </div>
        )}
        <div className="cbp-ai-insight">{insightModal.product?.ai_insight}</div>
        <Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0, fontSize: 12 }}>
          该洞察由 CB-Platform 智能引擎基于市场搜索量、竞品库存、评论情感、销量趋势等多维信号综合生成,仅供运营决策参考。
        </Paragraph>
      </Modal>

      <Modal
        title={formModal.mode === 'create' ? '新建选品' : '编辑选品'}
        open={formModal.open}
        onCancel={() => {
          setFormModal({ open: false, mode: 'create' });
          form.resetFields();
        }}
        onOk={handleSubmit}
        confirmLoading={submitting}
        okText={formModal.mode === 'create' ? '确认创建' : '保存修改'}
        cancelText="取消"
        width={720}
        destroyOnClose
      >
        <Form form={form} layout="vertical" preserve={false} style={{ marginTop: 8 }}>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="sku"
              label="SKU"
              rules={[{ required: true, message: '请输入 SKU' }]}
              style={{ flex: 1, marginBottom: 16 }}
            >
              <Input placeholder="如 PC-HAIR-001" disabled={formModal.mode === 'edit'} maxLength={64} />
            </Form.Item>
            <Form.Item
              name="name"
              label="商品名称"
              rules={[{ required: true, message: '请输入商品名称' }]}
              style={{ flex: 2, marginBottom: 16 }}
            >
              <Input placeholder="请输入商品名称" maxLength={200} />
            </Form.Item>
          </Space>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item name="category" label="类目" style={{ flex: 1, marginBottom: 16 }}>
              <Select options={CATEGORY_OPTIONS} placeholder="选择类目" allowClear />
            </Form.Item>
            <Form.Item name="stage" label="阶段" style={{ flex: 1, marginBottom: 16 }}>
              <Select options={STAGE_OPTIONS} placeholder="选择阶段" allowClear />
            </Form.Item>
            <Form.Item name="platform" label="平台" style={{ flex: 1, marginBottom: 16 }}>
              <Select options={PLATFORM_OPTIONS} placeholder="选择平台" allowClear />
            </Form.Item>
          </Space>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item name="target_market" label="目标市场" style={{ flex: 1, marginBottom: 16 }}>
              <Select options={MARKET_OPTIONS} placeholder="选择市场" allowClear />
            </Form.Item>
            <Form.Item name="list_price" label="上架价 (USD)" style={{ flex: 1, marginBottom: 16 }}>
              <InputNumber min={0} precision={2} style={{ width: '100%' }} placeholder="0.00" />
            </Form.Item>
            <Form.Item name="est_cost_price" label="预估成本价" style={{ flex: 1, marginBottom: 16 }}>
              <InputNumber min={0} precision={2} style={{ width: '100%' }} placeholder="0.00" />
            </Form.Item>
          </Space>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item name="monthly_sales" label="月销量" style={{ flex: 1, marginBottom: 16 }}>
              <InputNumber min={0} precision={0} style={{ width: '100%' }} placeholder="0" />
            </Form.Item>
            <Form.Item name="tags" label="标签" style={{ flex: 2, marginBottom: 16 }}>
              <Input placeholder="多个标签用英文逗号分隔，如 高毛利,爆款潜力" maxLength={200} />
            </Form.Item>
          </Space>
          <Form.Item name="remark" label="备注" style={{ marginBottom: 0 }}>
            <Input.TextArea rows={3} placeholder="选填备注信息" maxLength={500} showCount />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default Products;
