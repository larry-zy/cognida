/**
 * 目录树选择的纯逻辑：由 <input webkitdirectory> 选出的扁平 File 列表重建多级目录树，
 * 并提供勾选/半选/汇总等派生状态计算。不依赖 Vue，可独立单测。
 */

/** 与后端知识库上传接口保持一致的文件类型/大小限制 */
export const ACCEPTED_KNOWLEDGE_EXTENSIONS = ['.txt', '.md', '.pdf', '.doc', '.docx']
export const MAX_KNOWLEDGE_FILE_SIZE = 10 * 1024 * 1024 // 10 MiB

export interface FileTreeNode {
  /** 该层级的文件/目录名 */
  name: string
  /** 从根目录起的相对路径，用作 :key */
  path: string
  isFile: boolean
  /** 仅文件节点存在；目录节点的勾选态由子节点派生，不单独存储 */
  file?: File
  children: FileTreeNode[]
  /** 仅文件节点使用：是否勾选。目录节点该字段无意义 */
  checked: boolean
}

function hasAcceptedExtension(fileName: string, extensions: string[]): boolean {
  const lower = fileName.toLowerCase()
  return extensions.some((ext) => lower.endsWith(ext))
}

export interface FilterFilesResult {
  accepted: File[]
  rejectedByType: File[]
  rejectedBySize: File[]
}

/** 按扩展名与大小过滤，用于文件夹批量选择场景（webkitdirectory 无法用 accept 过滤） */
export function filterAcceptableFiles(
  files: File[],
  opts: { extensions?: string[]; maxSize?: number } = {}
): FilterFilesResult {
  const extensions = opts.extensions ?? ACCEPTED_KNOWLEDGE_EXTENSIONS
  const maxSize = opts.maxSize ?? MAX_KNOWLEDGE_FILE_SIZE
  const accepted: File[] = []
  const rejectedByType: File[] = []
  const rejectedBySize: File[] = []

  for (const file of files) {
    if (!hasAcceptedExtension(file.name, extensions)) {
      rejectedByType.push(file)
      continue
    }
    if (file.size > maxSize) {
      rejectedBySize.push(file)
      continue
    }
    accepted.push(file)
  }

  return { accepted, rejectedByType, rejectedBySize }
}

/**
 * 由扁平 File 列表（webkitRelativePath 形如 "父目录/子目录/a.md"）重建目录树。
 * 返回的根节点本身不代表任何真实目录，其 children 才是选中文件夹下的顶层节点。
 * 默认全部勾选，符合"选中父目录后底下全选中，需要再手动取消"的交互预期。
 */
export function buildFileTree(files: File[]): FileTreeNode {
  const root: FileTreeNode = { name: '', path: '', isFile: false, children: [], checked: false }

  // 构建期用 Map 做每层的 O(1) 查重，避免大文件夹（同层成百上千文件）时
  // Array.find 逐个比对退化成 O(n²)——批量上传的目标场景恰恰是这种大文件夹
  const childIndex = new Map<FileTreeNode, Map<string, FileTreeNode>>()

  function getOrCreateChild(
    parent: FileTreeNode,
    name: string,
    isFile: boolean,
    path: string,
    file?: File
  ): FileTreeNode {
    let index = childIndex.get(parent)
    if (!index) {
      index = new Map()
      childIndex.set(parent, index)
    }
    const key = `${isFile ? 'f' : 'd'}:${name}`
    let child = index.get(key)
    if (!child) {
      child = { name, path, isFile, children: [], checked: true, ...(isFile ? { file } : {}) }
      index.set(key, child)
      parent.children.push(child)
    }
    return child
  }

  for (const file of files) {
    const relativePath = file.webkitRelativePath || file.name
    const parts = relativePath.split('/').filter(Boolean)
    let cursor = root

    parts.forEach((part, idx) => {
      const isLastPart = idx === parts.length - 1
      const pathSoFar = parts.slice(0, idx + 1).join('/')
      cursor = getOrCreateChild(cursor, part, isLastPart, pathSoFar, isLastPart ? file : undefined)
    })
  }

  return root
}

/** 勾选/取消勾选某节点及其全部子孙文件节点 */
export function setSubtreeChecked(node: FileTreeNode, checked: boolean): void {
  if (node.isFile) {
    node.checked = checked
    return
  }
  node.children.forEach((child) => setSubtreeChecked(child, checked))
}

/** 该节点下的文件是否全部勾选（目录节点由子节点派生；空目录视为未全选） */
export function isNodeFullyChecked(node: FileTreeNode): boolean {
  if (node.isFile) return node.checked
  if (node.children.length === 0) return false
  return node.children.every(isNodeFullyChecked)
}

/** 该节点下的文件是否全部未勾选（空目录视为未选中） */
export function isNodeFullyUnchecked(node: FileTreeNode): boolean {
  if (node.isFile) return !node.checked
  if (node.children.length === 0) return true
  return node.children.every(isNodeFullyUnchecked)
}

/** 目录节点的半选态：子孙文件勾选情况不一致 */
export function isNodeIndeterminate(node: FileTreeNode): boolean {
  if (node.isFile) return false
  return !isNodeFullyChecked(node) && !isNodeFullyUnchecked(node)
}

/** 收集该节点下所有已勾选的文件（按原始遍历顺序） */
export function collectCheckedFiles(node: FileTreeNode): File[] {
  if (node.isFile) {
    return node.checked && node.file ? [node.file] : []
  }
  return node.children.flatMap(collectCheckedFiles)
}

/** 该节点下的文件总数 */
export function countFiles(node: FileTreeNode): number {
  if (node.isFile) return 1
  return node.children.reduce((sum, child) => sum + countFiles(child), 0)
}

/** 该节点下的文件总大小（字节） */
export function sumFileSize(node: FileTreeNode): number {
  if (node.isFile) return node.file?.size ?? 0
  return node.children.reduce((sum, child) => sum + sumFileSize(child), 0)
}
