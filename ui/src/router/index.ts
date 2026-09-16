import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'

/**
 * 路由元信息单一数据源：页面标题、菜单名、图标全部收敛到 meta。
 * NavMenu 遍历路由渲染（跳过 hidden），DefaultLayout 从 meta 取面包屑标题。
 * hidden：后端能力未就绪时隐藏入口（FaaS：后端无此模块）。
 */
const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'dashboard',
    component: () => import('../pages/Dashboard.vue'),
    meta: { title: '监控大盘' }
  },
  {
    path: '/databases',
    name: 'databases',
    component: () => import('../pages/Databases.vue'),
    meta: { title: '数据库' }
  },
  {
    path: '/sql',
    name: 'sql',
    component: () => import('../pages/SqlConsole.vue'),
    meta: { title: 'SQL 控制台' }
  },
  {
    path: '/data',
    name: 'data',
    component: () => import('../pages/DataManager.vue'),
    meta: { title: '数据管理' }
  },
  {
    path: '/s3',
    name: 's3',
    component: () => import('../pages/S3Manager.vue'),
    meta: { title: 'S3 对象存储' }
  },
  {
    path: '/faas',
    name: 'faas',
    component: () => import('../pages/FaaSManager.vue'),
    meta: { title: '云函数', hidden: true }
  },
  {
    path: '/llm',
    name: 'llm',
    component: () => import('../pages/LlmManager.vue'),
    meta: { title: 'LLM 对话' }
  },
  {
    path: '/settings',
    name: 'settings',
    component: () => import('../pages/Settings.vue'),
    meta: { title: '设置' }
  },
  {
    path: '/logs',
    name: 'logs',
    component: () => import('../pages/Logs.vue'),
    meta: { title: '日志管理' }
  }
]

export default createRouter({
  history: createWebHistory(),
  routes
})
