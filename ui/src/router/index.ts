import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import DefaultLayout from '../layouts/DefaultLayout.vue'
import DocsLayout from '../layouts/DocsLayout.vue'

/**
 * 路由元信息单一数据源：页面标题、菜单名、图标全部收敛到 meta。
 * NavMenu 遍历路由渲染（跳过 hidden），DefaultLayout 从 meta 取面包屑标题。
 * hidden：文档站等非控制台入口不进侧栏。
 * `/settings` 重定向到 `/console/agents?settings=1` 由 DefaultLayout 打开全局 SbModal。
 *
 * titleKey 是 i18n 消息键（locales/zh-CN.ts 的 nav.* 命名空间），
 * 由 NavMenu 与 DefaultLayout 的 t() 消费；这里不再存放中文字面量。
 *
 * 主页、控制台与文档站拆布局：App.vue 只挂 <router-view />，
 * 控制台子路由走 DefaultLayout（侧栏 + 项目切换），文档子路由走 DocsLayout。
 */
const legacyConsolePaths = [
  'databases', 'key-value', 's3', 'gofunctions', 'cron-jobs', 'sandboxes', 'agents', 'logs', 'users'
]

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('../pages/Home.vue'),
    meta: { titleKey: 'nav.home', hidden: true },
    beforeEnter: (to) => to.query.settings == null ? true : ({
      name: 'agents',
      query: to.query,
      hash: to.hash
    })
  },
  {
    path: '/console',
    component: DefaultLayout,
    children: [
      {
        path: '',
        redirect: (to) => ({ name: 'agents', query: to.query, hash: to.hash })
      },
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('../pages/Dashboard.vue'),
        meta: { titleKey: 'nav.dashboard' }
      },
      {
        path: 'databases',
        name: 'databases',
        component: () => import('../pages/Databases.vue'),
        meta: { titleKey: 'nav.databases' }
      },
      { path: 'sql', redirect: (to) => ({ name: 'databases', query: to.query, hash: to.hash }) },
      {
        path: 'data',
        name: 'data-browser',
        component: () => import('../pages/DataBrowser.vue'),
        meta: { titleKey: 'nav.dataBrowser' }
      },
      {
        path: 'api-preview',
        name: 'api-preview',
        component: () => import('../pages/ApiPreview.vue'),
        meta: { titleKey: 'nav.apiPreview' }
      },
      {
        path: 'key-value',
        name: 'key-value',
        component: () => import('../pages/KeyValue.vue'),
        meta: { titleKey: 'nav.keyValue' }
      },
      {
        path: 's3',
        name: 's3',
        component: () => import('../pages/S3Manager.vue'),
        meta: { titleKey: 'nav.s3' }
      },
      {
        path: 'gofunctions',
        name: 'gofunctions',
        component: () => import('../pages/GoFunctions.vue'),
        meta: { titleKey: 'nav.gofunctions' }
      },
      {
        path: 'function-triggers',
        name: 'function-triggers',
        component: () => import('../pages/FunctionTriggers.vue'),
        meta: { titleKey: 'nav.functionTriggers' }
      },
      {
        path: 'cron-jobs',
        name: 'cron-jobs',
        component: () => import('../pages/CronJobs.vue'),
        meta: { titleKey: 'nav.cronJobs' }
      },
      {
        path: 'sandboxes',
        name: 'sandboxes',
        component: () => import('../pages/Sandboxes.vue'),
        meta: { titleKey: 'nav.sandboxes' }
      },
      {
        path: 'agents',
        name: 'agents',
        component: () => import('../pages/AgentManager.vue'),
        meta: { titleKey: 'nav.agents' }
      },
      { path: 'llm', redirect: (to) => ({ name: 'agents', query: to.query, hash: to.hash }) },
      {
        path: 'settings',
        name: 'settings',
        redirect: (to) => ({
          name: 'agents',
          query: { ...to.query, settings: typeof to.query.settings === 'string' ? to.query.settings : '1' },
          hash: to.hash
        })
      },
      {
        path: 'logs',
        name: 'logs',
        component: () => import('../pages/Logs.vue'),
        meta: { titleKey: 'nav.logs' }
      },
      {
        path: 'users',
        name: 'users',
        component: () => import('../pages/Users.vue'),
        meta: { titleKey: 'nav.users', requiresRole: 'superadminl1' }
      }
    ]
  },
  ...legacyConsolePaths.map((path): RouteRecordRaw => ({
    path: `/${path}`,
    redirect: (to) => ({ path: `/console/${path}`, query: to.query, hash: to.hash })
  })),
  ...['sql', 'data', 'llm', 'settings'].map((path): RouteRecordRaw => ({
    path: `/${path}`,
    redirect: (to) => ({ path: `/console/${path}`, query: to.query, hash: to.hash })
  })),
  {
    path: '/docs',
    component: DocsLayout,
    meta: { titleKey: 'nav.docs', hidden: true },
    children: [
      {
        path: '',
        name: 'docs',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { titleKey: 'nav.docs', hidden: true }
      },
      {
        path: ':module',
        name: 'docs-module',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { titleKey: 'nav.docs', hidden: true }
      },
      {
        path: ':module/:slug',
        name: 'docs-page',
        component: () => import('../pages/DocsWiki.vue'),
        meta: { titleKey: 'nav.docs', hidden: true }
      }
    ]
  }
]

export default createRouter({
  history: createWebHistory(),
  routes
})
