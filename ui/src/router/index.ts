import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import DefaultLayout from '../layouts/DefaultLayout.vue'
import DocsLayout from '../layouts/DocsLayout.vue'

/**
 * 路由元信息单一数据源：页面标题、菜单名、图标全部收敛到 meta。
 * NavMenu 遍历路由渲染（跳过 hidden），DefaultLayout 从 meta 取面包屑标题。
 * hidden：文档站等非控制台入口不进侧栏。
 * `/settings` 不再是全页，redirect 到 `/?settings=1` 由 DefaultLayout 打开全局 SbModal。
 *
 * 控制台与文档站拆布局：App.vue 只挂 <router-view />，
 * 控制台子路由走 DefaultLayout（侧栏 + 项目切换），文档子路由走 DocsLayout。
 */
const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: DefaultLayout,
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('../pages/Dashboard.vue'),
        meta: { title: '监控大盘' }
      },
      {
        path: 'databases',
        name: 'databases',
        component: () => import('../pages/Databases.vue'),
        meta: { title: '数据库管理' }
      },
      { path: 'sql', redirect: '/databases' },
      { path: 'data', redirect: '/databases' },
      {
        path: 's3',
        name: 's3',
        component: () => import('../pages/S3Manager.vue'),
        meta: { title: 'S3 对象存储' }
      },
      {
        path: 'gofunctions',
        name: 'gofunctions',
        component: () => import('../pages/GoFunctions.vue'),
        meta: { title: '云函数' }
      },
      {
        path: 'cron-jobs',
        name: 'cron-jobs',
        component: () => import('../pages/CronJobs.vue'),
        meta: { title: '定时任务' }
      },
      {
        path: 'agents',
        name: 'agents',
        component: () => import('../pages/AgentManager.vue'),
        meta: { title: '云 Agent' }
      },
      { path: 'llm', redirect: '/agents' },
      {
        path: 'settings',
        name: 'settings',
        redirect: (to) => ({
          path: '/',
          query: { ...to.query, settings: typeof to.query.settings === 'string' ? to.query.settings : '1' }
        })
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('../pages/Logs.vue'),
        meta: { title: '日志管理' }
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('../pages/Users.vue'),
        meta: { title: '用户管理', requiresRole: 'superadminl1' }
      }
    ]
  },
  {
    path: '/docs',
    component: DocsLayout,
    meta: { title: '使用文档', hidden: true },
    children: [
      {
        path: '',
        name: 'docs',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { title: '使用文档', hidden: true }
      },
      {
        path: ':module',
        name: 'docs-module',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { title: '使用文档', hidden: true }
      },
      {
        path: ':module/:slug',
        name: 'docs-page',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { title: '使用文档', hidden: true }
      }
    ]
  }
]

export default createRouter({
  history: createWebHistory(),
  routes
})
