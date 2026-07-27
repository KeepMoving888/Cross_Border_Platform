/**
 * 页面容器:统一企业级页头、间距与内容区
 */
import React from 'react';
import { PageContainer as ProPageContainer } from '@ant-design/pro-components';
import type { PageContainerProps } from '@ant-design/pro-components';

const PageContainer: React.FC<PageContainerProps> = ({ children, ...rest }) => {
  return (
    <ProPageContainer
      ghost
      header={{
        title: rest.title || '',
        breadcrumb: rest.breadcrumb,
        ...rest.header,
      }}
      {...rest}
    >
      <div style={{ minHeight: 'calc(100vh - 180px)' }}>{children}</div>
    </ProPageContainer>
  );
};

export default PageContainer;
