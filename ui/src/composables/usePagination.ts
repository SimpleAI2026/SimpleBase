import type { TablePaginationConfig } from 'ant-design-vue'

/** 表格统一分页配置（替代 3 个页面复制的 { pageSize: 10, size: 'small', showTotal }） */
export function usePagination(): TablePaginationConfig {
  return {
    pageSize: 10,
    size: 'small',
    showTotal: (t: number) => `共 ${t} 条`
  }
}
