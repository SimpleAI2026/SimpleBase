import Layout from '@/layout'

export const constantRoutes = [
  {
    path: '/redirect',
    component: Layout,
    hidden: true,
    children: [
      {
        path: '/redirect/:path(.*)',
        component: () => import('@/views/redirect/index')
      }
    ]
  },
  {
    path: '/login',
    component: () => import('@/views/account/sign-in'),
    hidden: true
  },
  {
    path: '/sign-up',
    component: () => import('@/views/account/sign-up'),
    hidden: true
  },
  {
    path: '/',
    component: Layout,
    redirect: "/homepage",
    hidden: true,
    children: [
      {
        path: 'homepage',
        component: () => import('@/views/homepage/index'),
        name: 'Homepage',
        meta: { title: '首页', icon: 'dashboard', noCache: true }
      }
    ]
  },
  {
    path: '/',
    component: Layout,
    redirect: "/document",
    hidden: true,
    children: [
      {
        path: 'document',
        component: () => import('@/views/document/index'),
        name: 'Document',
        meta: { title: '文档', icon: 'dashboard', noCache: true }
      }
    ]
  },
  {
    path: '/applications',
    component: Layout,
    hidden: true,
    children: [
      {
        path: '',
        component: () => import('@/views/application/index'),
        name: 'Application',
        meta: { title: '我的应用', icon: 'dashboard', noCache: true }
      }
    ]
  },
  {
    path: '/404',
    component: () => import('@/views/error-page/404'),
    hidden: true
  },
  // 404 page must be placed at the end !!!
  { path: '*', redirect: '/404', hidden: true }
]
