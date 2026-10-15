export interface NavigationItem { path: string; title: string; icon: string }
export interface NavigationGroup { title: string; items: NavigationItem[] }

export const navigationGroups: NavigationGroup[] = [
  { title: '工作流程', items: [
    { path: '/home', title: '工作总览', icon: 'home' },
    { path: '/tasks', title: '任务规划', icon: 'trace' },
    { path: '/collection-tasks', title: '网页采集', icon: 'collection' },
    { path: '/datasource', title: '数据源', icon: 'datasource' },
    { path: '/pipelines', title: '处理与发布', icon: 'trace' },
    { path: '/experts', title: '独立专家', icon: 'quality' }
  ] },
  { title: '知识与分析', items: [
    { path: '/knowledge', title: '知识库', icon: 'knowledge' },
    { path: '/kb-assistant', title: '知识检索与问答', icon: 'kb' },
    { path: '/analysis', title: '资产分析', icon: 'ai' },
    { path: '/analysis/chat', title: '数据对话', icon: 'ai' },
    { path: '/research', title: '深度研究', icon: 'kb' },
    { path: '/subscriptions', title: '报告与看护', icon: 'evaluation' },
    { path: '/graphs', title: '知识图谱', icon: 'graph' }
  ] },
  { title: '治理与质量', items: [
    { path: '/catalog', title: '数据目录', icon: 'datasource' },
    { path: '/quality', title: '数据质量', icon: 'quality' },
    { path: '/labeling', title: '标注与打标', icon: 'knowledge' },
    { path: '/features', title: '特征工程', icon: 'datasets' },
    { path: '/semantic-models', title: '语义建模', icon: 'graph' },
    { path: '/evaluation', title: 'Agent 测评', icon: 'evaluation' },
    { path: '/datasets', title: '评测数据集', icon: 'datasets' },
    { path: '/suggestions', title: '反馈与改进', icon: 'audit' }
  ] },
  { title: '系统与个人', items: [
    { path: '/memories', title: '长期记忆', icon: 'knowledge' },
    { path: '/audit-logs', title: '审计日志', icon: 'audit' },
    { path: '/traces', title: '调用链追踪', icon: 'trace' },
    { path: '/settings', title: '设置', icon: 'settings' }
  ] }
]

export function activeNavigationPath(path: string): string | undefined {
  const canonical = path.replace(/^\/data-agent(?=\/|$)/, '/analysis/chat')
  return navigationGroups.flatMap(group => group.items)
    .filter(item => canonical === item.path || canonical.startsWith(item.path + '/'))
    .sort((a, b) => b.path.length - a.path.length)[0]?.path
}
