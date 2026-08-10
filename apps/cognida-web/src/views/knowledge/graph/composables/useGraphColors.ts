// 图谱配色 composable —— 拆分自 GraphView.vue〔M14〕
// 职责：实体类型 → 钢青灰主色族的映射（月夜台账配色）。
// Canvas/D3 内部取不到 CSS 变量，直接写字面色值。
//
// 说明：动态类型映射（_dynamicTypeMap / _paletteIdx）改为 composable 实例内私有状态，
// GraphView 单实例使用，行为与原模块级单例完全一致。

/**
 * 实体类型 → 钢青灰主色族（按顺序循环）
 * #62758a 服务
 * #56687c 技术组件
 * #4a5b6e 存储
 * #3d4a5c 文档/其他
 */
const TYPE_PALETTE: string[] = [
  '#62758a',
  '#56687c',
  '#4a5b6e',
  '#3d4a5c'
]

// 已知实体类型固定映射
const typeColorMap: Record<string, string> = {
  // 服务类
  Department:   '#62758a',
  person:       '#62758a',
  organization: '#62758a',
  // 技术组件类
  Module:       '#56687c',
  Technology:   '#56687c',
  concept:      '#56687c',
  // 存储类
  Product:      '#4a5b6e',
  location:     '#4a5b6e',
  // 文档/其他
  Other:        '#3d4a5c',
  other:        '#3d4a5c'
}

export function useGraphColors() {
  // 动态分配：未在上表中的类型按出现顺序取 palette
  const _dynamicTypeMap: Record<string, string> = {}
  let _paletteIdx = 0

  function getTypeColor(entityType: string): string {
    if (typeColorMap[entityType]) return typeColorMap[entityType]
    if (_dynamicTypeMap[entityType]) return _dynamicTypeMap[entityType]
    const color = TYPE_PALETTE[_paletteIdx % TYPE_PALETTE.length]
    _paletteIdx++
    _dynamicTypeMap[entityType] = color
    return color
  }

  // vis-network node color 对象：正常 fill、选中时银青、hover 边框微亮
  // 返回值在调用侧以 `as any` 交给 vis-network（其颜色对象类型宽松）
  function buildVisNodeColor(baseColor: string) {
    return {
      background: baseColor,
      border: baseColor,
      highlight: {
        background: '#9cb4cd',
        border: '#9cb4cd'
      },
      hover: {
        background: baseColor,
        border: '#7f92a8'
      }
    }
  }

  return { getTypeColor, buildVisNodeColor }
}
