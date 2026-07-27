/**
 * 通用状态标签:根据状态字典返回带颜色的 Tag
 */
import React from 'react';
import { Tag } from 'antd';
import type { TagProps } from 'antd';
import type { PresetColor } from '@/utils/constants';

interface StatusTagProps extends Omit<TagProps, 'color'> {
  status: string;
  map: Record<string, { label: string; color: PresetColor }>;
  /** 是否使用 Badge 风格(用于采购单状态等) */
  badge?: boolean;
}

const StatusTag: React.FC<StatusTagProps> = ({ status, map, badge = false, ...rest }) => {
  const item = map[status];
  if (!item) {
    return <Tag {...rest}>{status}</Tag>;
  }
  if (badge) {
    return (
      <Tag color={item.color} {...rest} style={{ borderRadius: 12, padding: '0 10px', ...rest.style }}>
        {item.label}
      </Tag>
    );
  }
  return (
    <Tag color={item.color} {...rest}>
      {item.label}
    </Tag>
  );
};

export default StatusTag;
