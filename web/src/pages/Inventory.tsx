/**
 * 库存管理:企业级分仓库清单 + 预警看板 + 安全库存提醒 + 写操作闭环
 */
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Card,
  Tag,
  Space,
  Row,
  Col,
  Alert,
  Typography,
  Button,
  Modal,
  Form,
  Input,
  InputNumber,
  Select,
  message,
} from 'antd';
import {
  WarningOutlined,
  ExclamationCircleOutlined,
  ShoppingOutlined,
  ReloadOutlined,
  CheckOutlined,
  EditOutlined,
} from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import PageContainer from '@/components/PageContainer';
import {
  listInventory,
  listInventoryAlerts,
  resolveInventoryAlert,
  adjustInventory,
  listWarehouses,
} from '@/api/inventory';
import { useAuthStore } from '@/store/auth';
import { WAREHOUSE_MAP } from '@/utils/constants';
import { formatCNY, formatNumber, formatDateTime } from '@/utils/format';
import type { Inventory, InventoryAlert } from '@/types/api';

const { Text } = Typography;

const ADJUST_TYPE_OPTIONS = [
  { value: 'inbound', label: '入库' },
  { value: 'outbound', label: '出库' },
  { value: 'adjust', label: '盘点调整' },
  { value: 'return', label: '退货入库' },
];

const FALLBACK_WAREHOUSE_OPTIONS = Object.entries(WAREHOUSE_MAP).map(([value, label]) => ({
  value: Number(value),
  label,
}));

const Inventory: React.FC = () => {
  const navigate = useNavigate();
  const actionRef = useRef<ActionType>(null);
  const hasRole = useAuthStore((s) => s.hasRole);
  const canWrite = hasRole('admin', 'manager', 'staff');

  const [alerts, setAlerts] = useState<InventoryAlert[]>([]);
  const [alertsLoading, setAlertsLoading] = useState(true);
  const [resolvingId, setResolvingId] = useState<number | null>(null);
  const [warehouseOptions, setWarehouseOptions] = useState(FALLBACK_WAREHOUSE_OPTIONS);
  const [warehouseNameMap, setWarehouseNameMap] = useState<Record<number, string>>({ ...WAREHOUSE_MAP });

  const [adjustOpen, setAdjustOpen] = useState(false);
  const [adjustSubmitting, setAdjustSubmitting] = useState(false);
  const [adjustForm] = Form.useForm();

  const loadAlerts = async (nameMap?: Record<number, string>) => {
    const map = nameMap || warehouseNameMap;
    setAlertsLoading(true);
    try {
      const res = await listInventoryAlerts();
      const list = (res?.list || []).map((a) => ({
        ...a,
        available_qty: a.available_qty ?? a.current_qty,
        safety_stock: a.safety_stock ?? a.threshold,
        shortage_qty: a.shortage_qty ?? Math.max(0, (a.threshold ?? 0) - (a.current_qty ?? 0)),
        level: a.level ?? ((a.current_qty ?? 0) <= 0 ? 'critical' : 'warning'),
        warehouse_name:
          a.warehouse_name || map[a.warehouse_id] || WAREHOUSE_MAP[a.warehouse_id] || `仓库#${a.warehouse_id}`,
        product_name: a.product_name || a.sku,
      }));
      setAlerts(list);
    } finally {
      setAlertsLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let nameMap: Record<number, string> = { ...WAREHOUSE_MAP };
      try {
        const list = await listWarehouses();
        if (!cancelled && Array.isArray(list) && list.length > 0) {
          const opts = list.map((w) => ({ value: w.id, label: w.name }));
          const map: Record<number, string> = {};
          list.forEach((w) => {
            map[w.id] = w.name;
          });
          setWarehouseOptions(opts);
          setWarehouseNameMap(map);
          nameMap = map;
        }
      } catch {
        // 回退 WAREHOUSE_MAP
      }
      if (!cancelled) {
        await loadAlerts(nameMap);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const criticalCount = alerts.filter((a) => a.level === 'critical').length;
  const warehouseCount = new Set(alerts.map((a) => a.warehouse_name)).size;
  const skuCount = new Set(alerts.map((a) => a.sku)).size;

  const warehouseFilterOptions = useMemo(() => warehouseOptions, [warehouseOptions]);

  const handleResolve = async (alert: InventoryAlert) => {
    setResolvingId(alert.id);
    try {
      await resolveInventoryAlert(alert.id);
      message.success(`已处理预警：${alert.sku}`);
      await loadAlerts();
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setResolvingId(null);
    }
  };

  const openAdjust = (record?: Inventory) => {
    adjustForm.resetFields();
    if (record) {
      adjustForm.setFieldsValue({
        warehouse_id: record.warehouse_id,
        sku: record.sku,
        quantity: undefined,
        type: 'adjust',
        remark: '',
      });
    } else {
      adjustForm.setFieldsValue({ type: 'inbound' });
    }
    setAdjustOpen(true);
  };

  const handleAdjust = async () => {
    const values = await adjustForm.validateFields();
    setAdjustSubmitting(true);
    try {
      await adjustInventory({
        warehouse_id: values.warehouse_id,
        sku: values.sku,
        quantity: values.quantity,
        type: values.type,
        remark: values.remark,
      });
      message.success('库存调整成功');
      setAdjustOpen(false);
      adjustForm.resetFields();
      await loadAlerts();
      actionRef.current?.reload();
    } catch {
      // 错误已由拦截器提示
    } finally {
      setAdjustSubmitting(false);
    }
  };

  const columns: ProColumns<Inventory>[] = [
    { title: 'SKU / 商品搜索', dataIndex: 'keyword', hideInTable: true },
    {
      title: 'SKU',
      dataIndex: 'sku',
      width: 140,
      copyable: true,
    },
    {
      title: '商品名称',
      dataIndex: 'product_name',
      width: 220,
      ellipsis: true,
      search: false,
    },
    {
      title: '仓库',
      dataIndex: 'warehouse_id',
      width: 180,
      valueType: 'select',
      fieldProps: { options: warehouseFilterOptions },
      render: (_, r) => (
        <Tag color="blue" style={{ borderRadius: 4 }}>
          {r.warehouse_name || warehouseNameMap[r.warehouse_id] || WAREHOUSE_MAP[r.warehouse_id] || `仓库#${r.warehouse_id}`}
        </Tag>
      ),
    },
    {
      title: '可用库存',
      dataIndex: 'available_qty',
      width: 110,
      align: 'right',
      search: false,
      sorter: true,
      render: (_, r) => {
        const ratio = r.safety_stock > 0 ? r.available_qty / r.safety_stock : 2;
        if (r.available_qty < r.safety_stock) {
          return (
            <Text type="danger" strong>
              {formatNumber(r.available_qty)}
            </Text>
          );
        }
        if (ratio < 1.3) {
          return (
            <Text type="warning" strong>
              {formatNumber(r.available_qty)}
            </Text>
          );
        }
        return <Text strong>{formatNumber(r.available_qty)}</Text>;
      },
    },
    {
      title: '锁定库存',
      dataIndex: 'locked_qty',
      width: 100,
      align: 'right',
      search: false,
      render: (_, r) => formatNumber(r.locked_qty),
    },
    {
      title: '在途库存',
      dataIndex: 'in_transit_qty',
      width: 100,
      align: 'right',
      search: false,
      render: (_, r) => formatNumber(r.in_transit_qty),
    },
    {
      title: '安全库存',
      dataIndex: 'safety_stock',
      width: 100,
      align: 'right',
      search: false,
      render: (_, r) => formatNumber(r.safety_stock),
    },
    {
      title: '单位成本',
      dataIndex: 'unit_cost',
      width: 110,
      align: 'right',
      search: false,
      render: (_, r) => formatCNY(r.unit_cost),
    },
    {
      title: '库存状态',
      width: 130,
      search: false,
      render: (_, r) => {
        if (r.available_qty < r.safety_stock) {
          return (
            <Tag color="red" style={{ borderRadius: 4 }}>
              <WarningOutlined /> 低于安全库存
            </Tag>
          );
        }
        const ratio = r.safety_stock > 0 ? r.available_qty / r.safety_stock : 2;
        if (ratio < 1.3) {
          return (
            <Tag color="orange" style={{ borderRadius: 4 }}>
              <ExclamationCircleOutlined /> 偏低预警
            </Tag>
          );
        }
        return (
          <Tag color="green" style={{ borderRadius: 4 }}>
            健康
          </Tag>
        );
      },
    },
    {
      title: '操作',
      valueType: 'option',
      width: 100,
      fixed: 'right',
      render: (_, r) =>
        canWrite
          ? [
              <Button
                key="adjust"
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={(e) => {
                  e.stopPropagation();
                  openAdjust(r);
                }}
              >
                调整
              </Button>,
            ]
          : [],
    },
  ];

  return (
    <PageContainer
      title="库存管理"
      breadcrumb={{}}
      subTitle=""
      extra={[
        <Button
          key="reload"
          icon={<ReloadOutlined />}
          onClick={() => {
            loadAlerts();
            actionRef.current?.reload();
          }}
        >
          刷新
        </Button>,
        canWrite ? (
          <Button key="adjust" icon={<EditOutlined />} onClick={() => openAdjust()}>
            调整库存
          </Button>
        ) : null,
        <Button key="purchase" type="primary" icon={<ShoppingOutlined />} onClick={() => navigate('/purchases?create=1')}>
          去创建采购
        </Button>,
      ].filter(Boolean)}
    >
      {alerts.length > 0 && (
        <Card
          className="cbp-alert-strip"
          title={
            <Space>
              <WarningOutlined style={{ color: '#cf1322' }} />
              <span>库存预警看板</span>
            </Space>
          }
          size="small"
          style={{ marginBottom: 16 }}
          loading={alertsLoading}
          extra={<Tag color="red">{alerts.length} 条预警</Tag>}
        >
          <Row gutter={[12, 12]}>
            {alerts.slice(0, 8).map((a) => (
              <Col xs={24} sm={12} xl={6} key={a.id}>
                <Alert
                  type={a.level === 'critical' ? 'error' : 'warning'}
                  showIcon
                  message={
                    <Space wrap>
                      <Text strong>{a.product_name}</Text>
                      <Tag>{a.sku}</Tag>
                      <Tag color={a.level === 'critical' ? 'red' : 'orange'} style={{ borderRadius: 4 }}>
                        {a.level === 'critical' ? '严重缺货' : '库存偏低'}
                      </Tag>
                    </Space>
                  }
                  description={
                    <div>
                      <Space split={<Text type="secondary">·</Text>} wrap style={{ marginBottom: 8 }}>
                        <Tag color="blue" style={{ borderRadius: 4, marginInlineEnd: 0 }}>
                          {a.warehouse_name}
                        </Tag>
                        <Text type="danger">
                          可用 {formatNumber(a.available_qty)} / 安全 {formatNumber(a.safety_stock)}
                        </Text>
                        {typeof a.shortage_qty === 'number' && a.shortage_qty > 0 && (
                          <Text type="secondary">缺口 {formatNumber(a.shortage_qty)}</Text>
                        )}
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {formatDateTime(a.created_at)}
                        </Text>
                      </Space>
                      <Space size={8}>
                        {canWrite && (
                          <Button
                            size="small"
                            type="primary"
                            ghost
                            icon={<CheckOutlined />}
                            loading={resolvingId === a.id}
                            onClick={() => handleResolve(a)}
                          >
                            处理
                          </Button>
                        )}
                        <Button
                          size="small"
                          icon={<ShoppingOutlined />}
                          onClick={() => navigate(`/purchases?create=1&sku=${encodeURIComponent(a.sku)}`)}
                        >
                          补货
                        </Button>
                      </Space>
                    </div>
                  }
                />
              </Col>
            ))}
          </Row>
        </Card>
      )}

      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile cbp-clickable-card" onClick={() => actionRef.current?.reload()}>
            <div className="label">预警总数</div>
            <div className="value" style={{ color: '#cf1322' }}>{alerts.length}</div>
            <div className="hint">低于安全库存条目</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">严重缺货</div>
            <div className="value" style={{ color: '#fa541c' }}>{criticalCount}</div>
            <div className="hint">建议优先补货</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile">
            <div className="label">涉及仓库</div>
            <div className="value" style={{ color: '#fa8c16' }}>{warehouseCount}</div>
            <div className="hint">跨仓风险分布</div>
          </div>
        </Col>
        <Col xs={12} md={6}>
          <div className="cbp-metric-tile cbp-clickable-card" onClick={() => navigate('/purchases?create=1')}>
            <div className="label">待补货 SKU</div>
            <div className="value" style={{ color: '#2F54EB' }}>{skuCount}</div>
            <div className="hint">点击进入采购管理</div>
          </div>
        </Col>
      </Row>

      <ProTable<Inventory>
        rowKey={(r) => `${r.warehouse_id}-${r.sku}`}
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1400 }}
        search={{ labelWidth: 80 }}
        cardBordered
        request={async (params) => {
          const res = await listInventory({
            page: params.current,
            page_size: params.pageSize,
            keyword: params.keyword,
            warehouse_id: params.warehouse_id as number | undefined,
          });
          return { data: res.list, total: res.total, success: true };
        }}
        pagination={{ pageSize: 10, showSizeChanger: true }}
        options={{ density: false, fullScreen: true, reload: true }}
        headerTitle="库存清单"
      />

      <Modal
        title="调整库存"
        open={adjustOpen}
        onCancel={() => {
          setAdjustOpen(false);
          adjustForm.resetFields();
        }}
        onOk={handleAdjust}
        confirmLoading={adjustSubmitting}
        destroyOnClose
        okText="确认调整"
        cancelText="取消"
        width={520}
      >
        <Form form={adjustForm} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item
            name="warehouse_id"
            label="仓库"
            rules={[{ required: true, message: '请选择仓库' }]}
          >
            <Select
              placeholder="请选择仓库"
              options={warehouseOptions}
              showSearch
              optionFilterProp="label"
            />
          </Form.Item>
          <Form.Item
            name="sku"
            label="SKU"
            rules={[{ required: true, message: '请输入 SKU' }]}
          >
            <Input placeholder="请输入 SKU" allowClear />
          </Form.Item>
          <Form.Item
            name="type"
            label="调整类型"
            rules={[{ required: true, message: '请选择调整类型' }]}
          >
            <Select options={ADJUST_TYPE_OPTIONS} placeholder="请选择类型" />
          </Form.Item>
          <Form.Item
            name="quantity"
            label="数量"
            rules={[
              { required: true, message: '请输入数量' },
              { type: 'number', min: 1, message: '数量须大于 0' },
            ]}
          >
            <InputNumber min={1} precision={0} style={{ width: '100%' }} placeholder="请输入数量" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={3} placeholder="可选，填写调整原因" maxLength={200} showCount />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default Inventory;
