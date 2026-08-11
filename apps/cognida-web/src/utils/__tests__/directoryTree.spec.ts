import { describe, it, expect } from 'vitest'
import {
  buildFileTree,
  filterAcceptableFiles,
  setSubtreeChecked,
  isNodeFullyChecked,
  isNodeFullyUnchecked,
  isNodeIndeterminate,
  collectCheckedFiles,
  countFiles,
  sumFileSize,
  MAX_KNOWLEDGE_FILE_SIZE
} from '@/utils/directoryTree'

/** 构造带 webkitRelativePath 的 File，模拟 <input webkitdirectory> 选择结果 */
function makeFile(relativePath: string, size = 10): File {
  const name = relativePath.split('/').pop() || relativePath
  const file = new File([new Uint8Array(size)], name)
  Object.defineProperty(file, 'webkitRelativePath', { value: relativePath })
  return file
}

describe('directoryTree/buildFileTree', () => {
  it('按 webkitRelativePath 重建多级目录树', () => {
    const files = [
      makeFile('docs/a.md'),
      makeFile('docs/sub/b.md'),
      makeFile('docs/sub/c.md')
    ]
    const root = buildFileTree(files)

    expect(root.children).toHaveLength(1)
    const docs = root.children[0]
    expect(docs.name).toBe('docs')
    expect(docs.isFile).toBe(false)
    // docs 下：a.md 一个文件节点 + sub 一个目录节点
    expect(docs.children).toHaveLength(2)

    const sub = docs.children.find((c) => c.name === 'sub')!
    expect(sub.isFile).toBe(false)
    expect(sub.children.map((c) => c.name).sort()).toEqual(['b.md', 'c.md'])
  })

  it('文件节点默认全部勾选', () => {
    const root = buildFileTree([makeFile('a.md'), makeFile('b.md')])
    expect(root.children.every((c) => c.checked)).toBe(true)
  })

  it('空文件列表返回无子节点的根', () => {
    const root = buildFileTree([])
    expect(root.children).toHaveLength(0)
  })

  it('同名文件与目录在同一层级不会互相覆盖', () => {
    // a 既是目录（a/x.md）又不会与文件重名，此处验证多层路径不会因 find 逻辑而错误合并
    const files = [makeFile('a/x.md'), makeFile('a/y.md')]
    const root = buildFileTree(files)
    expect(root.children).toHaveLength(1)
    expect(root.children[0].children).toHaveLength(2)
  })
})

describe('directoryTree/filterAcceptableFiles', () => {
  it('按扩展名过滤不支持的文件类型', () => {
    const files = [makeFile('a.md'), makeFile('b.exe'), makeFile('c.pdf')]
    const result = filterAcceptableFiles(files)
    expect(result.accepted.map((f) => f.name)).toEqual(['a.md', 'c.pdf'])
    expect(result.rejectedByType.map((f) => f.name)).toEqual(['b.exe'])
  })

  it('按大小过滤超过限制的文件', () => {
    const small = makeFile('small.md', 10)
    const big = makeFile('big.md', MAX_KNOWLEDGE_FILE_SIZE + 1)
    const result = filterAcceptableFiles([small, big])
    expect(result.accepted.map((f) => f.name)).toEqual(['small.md'])
    expect(result.rejectedBySize.map((f) => f.name)).toEqual(['big.md'])
  })

  it('扩展名比较忽略大小写', () => {
    const result = filterAcceptableFiles([makeFile('A.MD')])
    expect(result.accepted).toHaveLength(1)
  })
})

describe('directoryTree/勾选与半选派生状态', () => {
  it('取消目录下某一文件后，目录呈半选态；全部取消后目录呈未选态', () => {
    const root = buildFileTree([makeFile('docs/a.md'), makeFile('docs/b.md')])
    const docs = root.children[0]
    const [a, b] = docs.children

    expect(isNodeFullyChecked(docs)).toBe(true)
    expect(isNodeIndeterminate(docs)).toBe(false)

    a.checked = false
    expect(isNodeIndeterminate(docs)).toBe(true)
    expect(isNodeFullyChecked(docs)).toBe(false)
    expect(isNodeFullyUnchecked(docs)).toBe(false)

    b.checked = false
    expect(isNodeFullyUnchecked(docs)).toBe(true)
    expect(isNodeIndeterminate(docs)).toBe(false)
  })

  it('setSubtreeChecked 对目录整体取消/勾选会联动全部子孙文件', () => {
    const root = buildFileTree([
      makeFile('docs/a.md'),
      makeFile('docs/sub/b.md'),
      makeFile('docs/sub/c.md')
    ])
    const docs = root.children[0]

    setSubtreeChecked(docs, false)
    expect(collectCheckedFiles(docs)).toHaveLength(0)

    setSubtreeChecked(docs, true)
    expect(collectCheckedFiles(docs)).toHaveLength(3)
  })
})

describe('directoryTree/汇总统计', () => {
  it('collectCheckedFiles 只返回勾选的文件，忽略取消勾选的', () => {
    const root = buildFileTree([makeFile('a.md', 5), makeFile('b.md', 7)])
    root.children[1].checked = false
    const checked = collectCheckedFiles(root)
    expect(checked.map((f) => f.name)).toEqual(['a.md'])
  })

  it('countFiles / sumFileSize 统计整棵树', () => {
    const root = buildFileTree([makeFile('docs/a.md', 5), makeFile('docs/sub/b.md', 7)])
    expect(countFiles(root)).toBe(2)
    expect(sumFileSize(root)).toBe(12)
  })
})
