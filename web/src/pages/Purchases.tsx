import React, { useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  Drawer,
  Descriptions,
  Button,
  Space,
  Tag,
  Typography,
  message,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  DatePicker,
  Card,
  Row,
  Col,
  Steps,
  Timeline,
  Spin,
  Alert,
  Statistic,
  List,
} from 'antd';
import {
  ExclamationCircleOutlined,
  ShoppingOutlined,
  TruckOutlined,
  PlusOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import dayjs from 'dayjs';
import PageContainer from '@/components/PageContainer';
import StatusTag from '@/components/StatusTag';
import { listPurchases, transitionPurchase, createPurchase, listSuppliers, listPurchaseStatusLogs } from '@/api/purchases';
import { runAIWorkflow } from '@/api/ai';
import { useAuthStore } from '@/store/auth';
import {
  PURCHASE_STATUS_MAP,
  PURCHASE_TRANSITIONS,
} from '@/utils/constants';
import { formatCNY, formatDate, formatDateTime, formatNumber } from '@/utils/format';
import type { PurchaseOrder, PurchaseStatus, PurchaseStatusLog, StatusEventOption, Supplier } from '@/types/api';

const { Text, Title } = Typography;

const STATUS_OPTIONS = Object.entries(PURCHASE_STATUS_MAP).map(([value, { label }]) => ({
  value: value as PurchaseStatus,
  label,
}));

const STATUS_ORDER: PurchaseStatus[] = [
  'inquiry',
  'quoting',
  'ordered',
  'tracking',
  'shipped',
  'received',
  'qc',
  'reconciling',
  'settled',
];

const StatusSteps: React.FC<{ status: PurchaseStatus; logs: PurchaseStatusLog[] }> = ({
  status,
  logs,
}) => {
  if (status === 'cancelled') {
    return (
      <Card size="small" bordered={false} style={{ background: '#fff2f0', border: '1px solid #ffccc7' }}>
        <Space>
          <Tag color="error">已取消</Tag>
          <Text type="secondary">该采购单已终止，不再进入后续履约节点</Text>
        </Space>
      </Card>
    );
  }
  const currentIdx = STATUS_ORDER.indexOf(status);
  // 取每个状态首次进入的时间(日志按 id DESC 返回,先出现的更早写入的会被后出现的覆盖,
  // 因此用 !has 判断保留最早一条;但后端返回 DESC,我们反转后按时间正序取首次)
  const statusTimeMap = new Map<string, string>();
  [...logs].reverse().forEach((log) => {
    if (!statusTimeMap.has(log.to_status)) {
      statusTimeMap.set(log.to_status, log.created_at);
    }
  });
  return (
    <Steps
      current={currentIdx}
      direction="horizontal"
      size="small"
      labelPlacement="vertical"
      items={STATUS_ORDER.map((s, idx) => {
        const time = statusTimeMap.get(s);
        const isPast = idx < currentIdx;
        const isCurrent = idx === currentIdx;
        let description = '';
        if (time) {
          description = formatDate(time);
        } else if (isCurrent) {
          description = '当前状态';
        } else if (!isPast) {
          description = '待进入';
        }
        return {
          title: PURCHASE_STATUS_MAP[s].label,
          description,
        };
      })}
    />
  );
};

const StatusLogsTimeline: React.FC<{
  logs: PurchaseStatusLog[];
  currentStatus: PurchaseStatus;
}> = ({ logs, currentStatus }) => {
  if (!logs.length) {
    return <Text type="secondary">暂无状态变更日志</Text>;
  }
  const currentIdx = STATUS_ORDER.indexOf(currentStatus);
  const remainingSteps =
    currentIdx >= 0 && currentStatus !== 'cancelled'
      ? STATUS_ORDER.slice(currentIdx + 1)
      : [];
  return (
    <div>
      {remainingSteps.length > 0 && (
        <div
          style={{
            marginBottom: 16,
            padding: '8px 12px',
            background: '#fafafa',
            borderRadius: 4,
          }}
        >
          <Text type="secondary" style={{ fontSize: 12 }}>
            距离完成还剩 {remainingSteps.length} 步
          </Text>
          <div style={{ marginTop: 6 }}>
            {remainingSteps.map((s) => (
              <Tag
                key={s}
                color={PURCHASE_STATUS_MAP[s].color}
                style={{ marginRight: 4, marginBottom: 4 }}
              >
                {PURCHASE_STATUS_MAP[s].label}
              </Tag>
            ))}
          </div>
        </div>
      )}
      <Timeline
        items={logs.map((log) => {
          const fromLabel = log.from_status
            ? PURCHASE_STATUS_MAP[log.from_status as PurchaseStatus]?.label || log.from_status
            : '创建';
          const toLabel =
            PURCHASE_STATUS_MAP[log.to_status as PurchaseStatus]?.label || log.to_status;
          const toColor = PURCHASE_STATUS_MAP[log.to_status as PurchaseStatus]?.color;
          return {
            color: log.to_status === 'cancelled' ? 'red' : 'blue',
            children: (
              <div>
                <Space size={4} wrap>
                  <Tag>{fromLabel}</Tag>
                  <span style={{ color: '#8c8c8c' }}>→</span>
                  <Tag color={toColor}>{toLabel}</Tag>
                </Space>
                <div style={{ fontSize: 12, color: '#8c8c8c', marginTop: 4 }}>
                  {formatDateTime(log.created_at)}
                  {log.operator_name ? ` · ${log.operator_name}` : ''}
                </div>
                {log.remark && (
                  <div style={{ fontSize: 12, color: '#595959', marginTop: 4 }}>
                    备注：{log.remark}
                  </div>
                )}
              </div>
            ),
          };
        })}
      />
    </div>
  );
};

const Purchases: React.FC = () => {
  const actionRef = useRef<ActionType>(null);
  const [searchParams, setSearchParams] = useSearchParams();
  const hasRole = useAuthStore((s) => s.hasRole);
  const canManage = hasRole('admin', 'manager');

  const [drawer, setDrawer] = useState<{ open: boolean; record?: PurchaseOrder }>({ open: false });
  const [aiNegotiation, setAiNegotiation] = useState<{
    price_reasonable: boolean;
    negotiation: string;
    quote_price: number;
    market_avg: number;
    delivery_risk: string;
    checklist: string[];
  } | null>(null);
  const [aiNegotiating, setAiNegotiating] = useState(false);
  const [transitModal, setTransitModal] = useState<{
    open: boolean;
    event?: StatusEventOption;
  }>({ open: false });
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [transitForm] = Form.useForm();
  const [createForm] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [suppliers, setSuppliers] = useState<Supplier[]>([]);
  const [suppliersLoading, setSuppliersLoading] = useState(false);
  const [statusLogs, setStatusLogs] = useState<PurchaseStatusLog[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);

  const loadSuppliers = async () => {
    setSuppliersLoading(true);
    try {
      const res = await listSuppliers({ page: 1, page_size: 200 });
      setSuppliers(res.list || []);
    } catch {
      setSuppliers([]);
    } finally {
      setSuppliersLoading(false);
    }
  };

  const loadStatusLogs = async (id: number) => {
    setLogsLoading(true);
    try {
      const logs = await listPurchaseStatusLogs(id);
      setStatusLogs(logs || []);
    } catch {
      setStatusLogs([]);
    } finally {
      setLogsLoading(false);
    }
  };

  const handleAiNegotiate = async (record: PurchaseOrder) => {
    setAiNegotiating(true);
    try {
      const result = await runAIWorkflow(2, {
        input: {
          order_no: record.order_no,
          unit_price: String(record.unit_price ?? ''),
          quantity: String(record.quantity ?? ''),
        },
      });
      const parsed = JSON.parse(result.output);
      const inner = parsed.parsed || parsed;
      setAiNegotiation({
        price_reasonable: inner.price_reasonable,
        negotiation: inner.negotiation || '',
        quote_price: Number(inner.quote_price || 0),
        market_avg: Number(inner.market_avg || 0),
        delivery_risk: inner.delivery_risk || 'medium',
        checklist: Array.isArray(inner.checklist) ? inner.checklist : [],
      });
      message.success('AI 谈判分析完成');
    } catch (err) {
      message.error(err instanceof Error ? err.message : 'AI 分析失败');
    } finally {
      setAiNegotiating(false);
    }
  };

  const openCreateModal = () => {
    createForm.resetFields();
    setCreateModalOpen(true);
    if (!suppliers.length) {
      loadSuppliers();
    }
  };

  useEffect(() => {
    if (searchParams.get('create') === '1' && canManage) {
      openCreateModal();
      const next = new URLSearchParams(searchParams);
      next.delete('create');
      setSearchParams(next, { replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 抽屉打开时拉取状态变更日志;关闭时清空,避免下一次打开闪现旧数据
  useEffect(() => {
    if (drawer.open && drawer.record?.id) {
      loadStatusLogs(drawer.record.id);
    } else {
      setStatusLogs([]);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drawer.open, drawer.record?.id]);

  const handleCreate = async () => {
    const values = await createForm.validateFields();
    setCreateSubmitting(true);
    try {
      await createPurchase({
        title: values.title || undefined,
        product_name: values.product_name,
        supplier_id: values.supplier_id,
        sku: values.sku || undefined,
        quantity: values.quantity,
        unit_price: values.unit_price,
        expected_date: values.expected_date
          ? dayjs(values.expected_date).format('YYYY-MM-DD')
          : undefined,
        payment_terms: values.payment_terms || undefined,
        remark: values.remark || undefined,
      });
      message.success('采购单创建成功');
      setCreateModalOpen(false);
      createForm.resetFields();
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setCreateSubmitting(false);
    }
  };

  const columns: ProColumns<PurchaseOrder>[] = [
    { title: '采购单号', dataIndex: 'order_no', width: 180, copyable: true },
    {
      title: '单据搜索',
      dataIndex: 'keyword',
      hideInTable: true,
    },
    {
      title: '单据主题',
      dataIndex: 'title',
      width: 220,
      ellipsis: true,
    },
    {
      title: '商品',
      dataIndex: 'product_name',
      width: 200,
      search: false,
      ellipsis: true,
    },
    { title: 'SKU', dataIndex: 'sku', width: 130, search: false, copyable: true },
    {
      title: '供应商',
      dataIndex: 'supplier_name',
      width: 150,
      search: false,
      ellipsis: true,
    },
    {
      title: '数量',
      dataIndex: 'quantity',
      width: 90,
      search: false,
      align: 'right',
      render: (_, r) => formatNumber(r.quantity),
    },
    {
      title: '采购金额',
      dataIndex: 'total_amount',
      width: 120,
      search: false,
      align: 'right',
      render: (_, r) => formatCNY(r.total_amount),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      valueType: 'select',
      fieldProps: { options: STATUS_OPTIONS },
      render: (_, r) => <StatusTag status={r.status} map={PURCHASE_STATUS_MAP} badge />,
    },
    {
      title: '预计到货',
      dataIndex: 'expected_date',
      width: 120,
      search: false,
      render: (_, r) => formatDate(r.expected_date),
    },
    {
      title: '操作',
      valueType: 'option',
      width: 90,
      fixed: 'right',
      render: (_, r) => [
        <Button
          key="detail"
          type="link"
          size="small"
          onClick={(e) => {
            e.stopPropagation();
            setDrawer({ open: true, record: r });
          }}
        >
          详情
        </Button>,
      ],
    },
  ];

  const handleConfirmTransition = async () => {
    const values = await transitForm.validateFields();
    if (!drawer.record || !transitModal.event) return;
    setSubmitting(true);
    try {
      await transitionPurchase(drawer.record.id, {
        event: transitModal.event.event,
        remark: values.remark,
      });
      message.success(`已执行:${transitModal.event.label}`);
      setTransitModal({ open: false });
      transitForm.resetFields();
      setDrawer({ open: false });
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setSubmitting(false);
    }
  };

  const supplierOptions = suppliers.map((s) => ({
    value: s.id,
    label: s.code ? `${s.name}（${s.code}）` : s.name,
  }));

  return (
    <PageContainer title="采购管理" breadcrumb={{}}>
      <ProTable<PurchaseOrder>
        rowKey="id"
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1500 }}
        search={{ labelWidth: 80 }}
        cardBordered
        request={async (params) => {
          const res = await listPurchases({
            page: params.current,
            page_size: params.pageSize,
            keyword: params.keyword,
            status: params.status as PurchaseStatus | undefined,
          });
          return { data: res.list, total: res.total, success: true };
        }}
        pagination={{ pageSize: 10, showSizeChanger: true }}
        onRow={(record) => ({
          onClick: () => setDrawer({ open: true, record }),
          style: { cursor: 'pointer' },
        })}
        options={{ density: false, fullScreen: true, reload: true }}
        headerTitle="采购单列表"
        toolBarRender={() =>
          canManage
            ? [
                <Button
                  key="create"
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={openCreateModal}
                >
                  新建采购单
                </Button>,
              ]
            : []
        }
      />

      <Drawer
        title={
          <Space>
            <ShoppingOutlined style={{ color: '#1677ff' }} />
            <span>采购单详情</span>
          </Space>
        }
        width={760}
        open={drawer.open}
        onClose={() => { setDrawer({ open: false }); setAiNegotiation(null); }}
        destroyOnClose
        extra={
          drawer.record && (
            <Tag color={PURCHASE_STATUS_MAP[drawer.record.status].color} style={{ borderRadius: 12 }}>
              当前状态:{PURCHASE_STATUS_MAP[drawer.record.status].label}
            </Tag>
          )
        }
      >
        {drawer.record && (
          <>
            <Row gutter={[12, 12]} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <div className="cbp-metric-tile">
                  <div className="label">采购总额</div>
                  <div className="value" style={{ color: '#1677ff', fontSize: 18 }}>
                    {formatCNY(drawer.record.total_amount)}
                  </div>
                </div>
              </Col>
              <Col span={8}>
                <div className="cbp-metric-tile">
                  <div className="label">采购数量</div>
                  <div className="value" style={{ fontSize: 18 }}>
                    {formatNumber(drawer.record.quantity)}
                  </div>
                </div>
              </Col>
              <Col span={8}>
                <div className="cbp-metric-tile">
                  <div className="label">预计到货</div>
                  <div className="value" style={{ fontSize: 16 }}>
                    {formatDate(drawer.record.expected_date)}
                  </div>
                </div>
              </Col>
            </Row>

            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="采购单号" span={2}>
                <Text copyable strong>
                  {drawer.record.order_no}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="单据主题" span={2}>
                {drawer.record.title}
              </Descriptions.Item>
              <Descriptions.Item label="商品名称" span={2}>
                {drawer.record.product_name}
              </Descriptions.Item>
              <Descriptions.Item label="SKU">{drawer.record.sku}</Descriptions.Item>
              <Descriptions.Item label="供应商">{drawer.record.supplier_name}</Descriptions.Item>
              <Descriptions.Item label="采购单价">{formatCNY(drawer.record.unit_price)}</Descriptions.Item>
              <Descriptions.Item label="实际到货">{formatDate(drawer.record.actual_date)}</Descriptions.Item>
              <Descriptions.Item label="物流单号" span={2}>
                {drawer.record.logistics_no ? (
                  <Text copyable>
                    <TruckOutlined style={{ marginRight: 6, color: '#1677ff' }} />
                    {drawer.record.logistics_no}
                  </Text>
                ) : (
                  <Text type="secondary">未填写</Text>
                )}
              </Descriptions.Item>
              <Descriptions.Item label="物流公司" span={2}>
                {drawer.record.logistics_company || <Text type="secondary">未填写</Text>}
              </Descriptions.Item>
            </Descriptions>

            <Title level={5} style={{ marginTop: 24, marginBottom: 12 }}>
              <Space>
                <RobotOutlined style={{ color: '#722ed1' }} />
                AI 谈判助手
              </Space>
            </Title>
            <Card size="small" bordered={false} style={{ background: '#faf5ff', border: '1px solid #d3adf7' }}>
              <Space direction="vertical" style={{ width: '100%' }} size={12}>
                <Button
                  type="primary"
                  icon={<RobotOutlined />}
                  loading={aiNegotiating}
                  onClick={() => drawer.record && handleAiNegotiate(drawer.record)}
                  style={{ background: '#722ed1', borderColor: '#722ed1' }}
                >
                  {aiNegotiation ? '重新分析' : '启动 AI 谈判分析'}
                </Button>
                {aiNegotiation && (
                  <>
                    <Alert
                      type={aiNegotiation.price_reasonable ? 'success' : 'warning'}
                      showIcon
                      message={aiNegotiation.price_reasonable ? '报价合理' : '报价偏高，建议继续谈判'}
                      description={aiNegotiation.negotiation}
                    />
                    <Row gutter={8}>
                      <Col span={8}>
                        <Statistic title="报价" prefix="¥" value={aiNegotiation.quote_price} precision={2} />
                      </Col>
                      <Col span={8}>
                        <Statistic title="市场均价" prefix="¥" value={aiNegotiation.market_avg} precision={2} />
                      </Col>
                      <Col span={8}>
                        <Statistic title="交付风险" value={aiNegotiation.delivery_risk === 'low' ? '低' : aiNegotiation.delivery_risk === 'medium' ? '中' : '高'} />
                      </Col>
                    </Row>
                    {aiNegotiation.checklist.length > 0 && (
                      <Card size="small" type="inner" title="执行检查清单">
                        <List
                          size="small"
                          dataSource={aiNegotiation.checklist}
                          renderItem={(item: string) => <List.Item>{item}</List.Item>}
                        />
                      </Card>
                    )}
                  </>
                )}
              </Space>
            </Card>

            <Title level={5} style={{ marginTop: 24, marginBottom: 12 }}>
              履约状态机
            </Title>
            <Card size="small" bordered={false} styles={{ body: { padding: '16px 12px' } }}>
              <StatusSteps status={drawer.record.status} logs={statusLogs} />
            </Card>

            <Title level={5} style={{ marginTop: 24, marginBottom: 12 }}>
              状态变更日志
            </Title>
            <Spin spinning={logsLoading}>
              <StatusLogsTimeline logs={statusLogs} currentStatus={drawer.record.status} />
            </Spin>

            <Title level={5} style={{ marginTop: 24, marginBottom: 12 }}>
              可执行操作
            </Title>
            {PURCHASE_TRANSITIONS[drawer.record.status].length > 0 ? (
              <Space wrap>
                {PURCHASE_TRANSITIONS[drawer.record.status].map((opt) => (
                  <Button
                    key={opt.event}
                    type={opt.event === 'cancel' ? 'default' : 'primary'}
                    danger={opt.event === 'cancel'}
                    onClick={() => setTransitModal({ open: true, event: opt })}
                  >
                    {opt.label} → {PURCHASE_STATUS_MAP[opt.next_status].label}
                  </Button>
                ))}
              </Space>
            ) : (
              <Text type="secondary">当前状态无后续可执行事件</Text>
            )}
          </>
        )}
      </Drawer>

      <Modal
        title={
          <Space>
            <ExclamationCircleOutlined style={{ color: '#faad14' }} />
            <span>确认流转:{transitModal.event?.label}</span>
          </Space>
        }
        open={transitModal.open}
        onCancel={() => {
          setTransitModal({ open: false });
          transitForm.resetFields();
        }}
        onOk={handleConfirmTransition}
        confirmLoading={submitting}
        okText="确认提交"
        cancelText="取消"
      >
        <Form form={transitForm} layout="vertical" preserve={false}>
          <Form.Item
            name="remark"
            label="流转备注"
            rules={[{ required: true, message: '请填写流转备注' }]}
          >
            <Input.TextArea
              rows={3}
              placeholder="例如:已与供应商确认发货时间,转入追踪阶段"
              maxLength={200}
              showCount
            />
          </Form.Item>
          {transitModal.event && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              提交后,状态将由「{PURCHASE_STATUS_MAP[drawer.record?.status ?? 'inquiry'].label}」变更为「
              {PURCHASE_STATUS_MAP[transitModal.event.next_status].label}」
            </Text>
          )}
        </Form>
      </Modal>

      <Modal
        title="新建采购单"
        open={createModalOpen}
        onCancel={() => {
          setCreateModalOpen(false);
          createForm.resetFields();
        }}
        onOk={handleCreate}
        confirmLoading={createSubmitting}
        okText="确认创建"
        cancelText="取消"
        width={680}
        destroyOnClose
      >
        <Form form={createForm} layout="vertical" preserve={false} style={{ marginTop: 8 }}>
          <Form.Item name="title" label="单据主题">
            <Input placeholder="选填，如 2026Q1 补货" maxLength={120} />
          </Form.Item>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="product_name"
              label="商品名称"
              rules={[{ required: true, message: '请输入商品名称' }]}
              style={{ flex: 2, marginBottom: 16 }}
            >
              <Input placeholder="请输入商品名称" maxLength={200} />
            </Form.Item>
            <Form.Item name="sku" label="SKU" style={{ flex: 1, marginBottom: 16 }}>
              <Input placeholder="选填 SKU" maxLength={64} />
            </Form.Item>
          </Space>
          <Form.Item
            name="supplier_id"
            label="供应商"
            rules={[{ required: true, message: '请选择供应商' }]}
          >
            <Select
              showSearch
              optionFilterProp="label"
              loading={suppliersLoading}
              options={supplierOptions}
              placeholder="请选择供应商"
              onDropdownVisibleChange={(open) => {
                if (open && !suppliers.length) loadSuppliers();
              }}
            />
          </Form.Item>
          <Space size={16} style={{ display: 'flex' }} align="start">
            <Form.Item
              name="quantity"
              label="采购数量"
              rules={[{ required: true, message: '请输入采购数量' }]}
              style={{ flex: 1, marginBottom: 16 }}
            >
              <InputNumber min={1} precision={0} style={{ width: '100%' }} placeholder="数量" />
            </Form.Item>
            <Form.Item
              name="unit_price"
              label="采购单价"
              rules={[{ required: true, message: '请输入采购单价' }]}
              style={{ flex: 1, marginBottom: 16 }}
            >
              <InputNumber min={0} precision={2} style={{ width: '100%' }} placeholder="0.00" />
            </Form.Item>
            <Form.Item name="expected_date" label="预计到货" style={{ flex: 1, marginBottom: 16 }}>
              <DatePicker style={{ width: '100%' }} placeholder="选择日期" />
            </Form.Item>
          </Space>
          <Form.Item name="payment_terms" label="付款条款">
            <Input placeholder="如 月结30天 / 预付30%" maxLength={100} />
          </Form.Item>
          <Form.Item name="remark" label="备注" style={{ marginBottom: 0 }}>
            <Input.TextArea rows={3} placeholder="选填备注信息" maxLength={500} showCount />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default Purchases;
