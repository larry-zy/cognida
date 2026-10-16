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
      {path:'/hf-datasets',name:'HFDatasets',component:()=>import('@/views/scenario/HFDatasetsView.vue'),meta:{title:'HF 数据集采集'}},
      {path:'/data-preparation',name:'DataPreparation',component:()=>import('@/views/scenario/DataPreparationView.vue'),meta:{title:'数据准备'}},
      {path:'/label-standards',name:'LabelStandards',component:()=>import('@/views/scenario/LabelStandardsView.vue'),meta:{title:'标注标准库'}},
      {path:'/sft-datasets',name:'SFTDatasets',component:()=>import('@/views/scenario/SFTDatasetsView.vue'),meta:{title:'SFT 数据集'}},
      {path: '/tasks', name: 'TaskWorkbench', component: () => import('@/views/tasks/TaskWorkbench.vue'), meta: {title: '任务规划'}},
      {path: "/memories", name: "MemoryWorkbench", component: () => import("@/views/memory/MemoryWorkbench.vue"), meta: {title: "长期记忆"}},
      {path: "/research", name: "DeepResearch", component: () => import("@/views/research/DeepResearchView.vue"), meta: {title: "深度研究"}},
      {path: "/analysis", name: "AnalysisWorkspace", component: () => import("@/views/lifecycle/LifecycleWorkbench.vue"), meta: {title: "数据分析"}},
      {path: "/suggestions", name: "Suggestions", component: () => import("@/views/governance/SuggestionsView.vue"), meta: {title: "反馈与改进"}},
      {path: '/feedback', name: 'Feedback', component: () => import('@/views/governance/FeedbackView.vue'), meta: {title: '反馈与返工'}},
      {path: '/catalog', name: 'CatalogWorkbench', component: () => import('@/views/catalog/CatalogWorkbench.vue'), meta: {title: '数据目录'}},
      {path: '/pipelines', name: 'PipelineWorkbench', component: () => import('@/views/lifecycle/LifecycleWorkbench.vue'), meta: {title: '数据流水线'}},
      {path: '/experts', name: 'ExpertWorkbench', component: () => import('@/views/lifecycle/LifecycleWorkbench.vue'), meta: {title: '专家工作台'}},
      {path: '/subscriptions', name: 'Subscriptions', component: () => import('@/views/governance/SubscriptionsView.vue'), meta: {title: '报告与指标看护'}},
      {path: '/labeling', name: 'Labeling', component: () => import('@/views/governance/LabelingWorkbench.vue'), meta: {title: '标注与打标'}},
      {path: '/features', name: 'Features', component: () => import('@/views/governance/FeaturesView.vue'), meta: {title: '特征工程'}},
      {path: '/sandbox', name: 'Sandbox', component: () => import('@/views/sandbox/SandboxView.vue'), meta: {title: '脚本执行'}},
      {path: '/warehouse', name: 'Warehouse', component: () => import('@/views/warehouse/WarehouseView.vue'), meta: {title: '数仓建模'}},
      {path: '/operations', name: 'Operations', component: () => import('@/views/operations/OperationsView.vue'), meta: {title: '受控 SQL 与 ETL'}},
      {path: '/api-sources', name: 'APISources', component: () => import('@/views/apisources/APISourcesView.vue'), meta: {title: 'API 采集'}},
      {path: '/drift', name: 'Drift', component: () => import('@/views/governance/DriftView.vue'), meta: {title: '漂移监控'}},
      {path: "/semantic-models", name: "SemanticModels", component: () => import("@/views/semantic/SemanticModels.vue"), meta: {title: "语义建模"}},
      {
        path: '/home',
        name: 'Home',
        component: () => import('@/views/home/HomeView.vue'),
        meta: { title: '工作总览' }
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
        path: '/kb-assistant',
        name: 'KBAssistant',
        component: () => import('@/views/ai/KBAssistantView.vue'),
        meta: { title: '知识库助手' }
      },
      {
        path: '/analysis/chat/:conversationId?',
        alias: '/data-agent/:conversationId?',
        name: 'DataAgent',
        component: () => import('@/views/ai/DataAgentView.vue'),
        meta: { title: '数据分析' }
      },
      {
        path: '/collection-tasks',
        name: 'CollectionTaskList',
        component: () => import('@/views/collection/CollectionTaskList.vue'),
        meta: { title: '网页采集任务' }
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
      {
        path: '/traces',
        name: 'TraceList',
        component: () => import('@/views/trace/TraceList.vue'),
        meta: { title: '调用链追踪' }
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
    document.title = `${to.meta.title} - Cognida`
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
