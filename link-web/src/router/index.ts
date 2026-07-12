import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/auth/Login.vue'),
    meta: { requiresAuth: false, title: '登录' }
  },
  {
    path: '/',
    name: 'Platform',
    component: () => import('@/views/platform/index.vue'),
    meta: { requiresAuth: true },
    redirect: '/home',
    children: [
      {
        path: '/home',
        name: 'Home',
        component: () => import('@/views/home/HomeView.vue'),
        meta: { title: '首页' }
      },
      {
        path: '/knowledge',
        name: 'KnowledgeList',
        component: () => import('@/views/knowledge/KnowledgeBaseList.vue'),
        meta: { title: '知识库列表' }
      },
      {
        path: '/knowledge/:id',
        name: 'KnowledgeDetail',
        component: () => import('@/views/knowledge/KnowledgeBase.vue'),
        meta: { title: '知识库详情' }
      },
      {
        path: '/graphs',
        name: 'GraphList',
        component: () => import('@/views/knowledge/GraphList.vue'),
        meta: { title: '知识图谱列表' }
      },
      {
        path: '/graphs/:kbId',
        name: 'GraphView',
        component: () => import('@/views/knowledge/GraphView.vue'),
        meta: { title: '知识图谱视图' }
      },
      {
        path: '/settings',
        name: 'Settings',
        component: () => import('@/views/settings/Settings.vue'),
        meta: { title: '设置' }
      },
      {
        path: '/agent',
        name: 'AgentList',
        component: () => import('@/views/agent/AgentList.vue'),
        meta: { title: 'Agent管理' }
      },
      {
        path: '/kb-assistant',
        name: 'KBAssistant',
        component: () => import('@/views/ai/KBAssistantView.vue'),
        meta: { title: '知识库助手' }
      },
      {
        path: '/data-agent',
        name: 'DataAgent',
        component: () => import('@/views/ai/DataAgentView.vue'),
        meta: { title: 'DaDa 数据智能体' }
      },
      {
        path: '/evaluation',
        name: 'EvaluationList',
        component: () => import('@/views/evaluation/EvaluationList.vue'),
        meta: { title: 'Agent 测评' }
      },
      {
        path: '/evaluation/:id',
        name: 'EvaluationDetail',
        component: () => import('@/views/evaluation/EvaluationDetail.vue'),
        meta: { title: '评测详情' }
      },
      {
        path: '/datasets',
        name: 'DatasetList',
        component: () => import('@/views/evaluation/DatasetList.vue'),
        meta: { title: '数据集管理' }
      },
      {
        path: '/quality',
        name: 'QualityCenter',
        component: () => import('@/views/quality/QualityCenter.vue'),
        meta: { title: '数据质量管理中心' }
      },
      {
        path: '/datasource',
        name: 'DatasourceList',
        component: () => import('@/views/datasource/DatasourceList.vue'),
        meta: { title: '数据源管理' }
      },
      {
        path: '/audit-logs',
        name: 'AuditLogList',
        component: () => import('@/views/audit/AuditLogList.vue'),
        meta: { title: '审计日志' }
      },
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    redirect: '/home'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth !== false)

  // 设置页面标题
  if (to.meta.title) {
    document.title = `${to.meta.title} - Link`
  }

  if (requiresAuth && !authStore.isAuthenticated) {
    // 需要认证但未登录，跳转到登录页
    next({ name: 'Login', query: { redirect: to.fullPath } })
  } else if (to.name === 'Login' && authStore.isAuthenticated) {
    // 已登录用户访问登录页，跳转到平台首页
    next({ path: '/' })
  } else {
    next()
  }
})

export default router
