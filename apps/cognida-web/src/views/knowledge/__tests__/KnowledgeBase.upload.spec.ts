import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import KnowledgeBase from '@/views/knowledge/KnowledgeBase.vue'
import toast from '@/utils/toast'
import { MAX_KNOWLEDGE_FILE_SIZE } from '@/utils/directoryTree'

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'kb-1' } }),
  useRouter: () => ({ push: vi.fn() })
}))

vi.mock('@/utils/toast', () => ({
  default: { success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }
}))

vi.mock('@/api/knowledge', () => ({
  knowledgeApi: {
    getDetail: vi.fn().mockResolvedValue({ data: { id: 'kb-1', name: '测试库' } }),
    getStats: vi.fn().mockResolvedValue({
      data: { knowledge_count: 0, chunk_count: 0, total_size: 0, entity_count: 0 }
    }),
    getKnowledgeList: vi.fn().mockResolvedValue({ data: { list: [], total: 0 } }),
    getKnowledgeStatus: vi.fn(),
    getChunks: vi.fn().mockResolvedValue({ data: { list: [], total: 0 } }),
    checkFile: vi.fn(),
    uploadFile: vi.fn(),
    deleteKnowledge: vi.fn(),
    rebuildGraph: vi.fn(),
    search: vi.fn(),
    update: vi.fn()
  }
}))

/** 构造指定大小的 File（用 size getter 伪造，避免真的分配 100MB 内存） */
function makeFile(name: string, size = 1024): File {
  const file = new File([new Uint8Array(1)], name)
  Object.defineProperty(file, 'size', { value: size })
  return file
}

/** 把一批 File 塞进真实 input 元素并触发 change，走组件真实的事件链路。
 *  jsdom 没有 DataTransfer，故用类数组对象伪造 FileList（Array.from 只要求 length + 下标） */
async function selectFiles(input: HTMLInputElement, files: File[]) {
  const fileList = { ...files, length: files.length, item: (i: number) => files[i] ?? null }
  Object.defineProperty(input, 'files', { value: fileList, configurable: true })
  input.dispatchEvent(new Event('change'))
  await Promise.resolve()
}

async function mountView() {
  const wrapper = mount(KnowledgeBase, {
    global: {
      // GraphView 依赖 vis-network 的真实尺寸，测试里只需壳；el-* 与 v-loading 由 main.ts 全局注册，这里补桩
      stubs: { GraphView: true, 'el-icon': true, RouterLink: true },
      directives: { loading: {} }
    },
    attachTo: document.body
  })
  await new Promise((r) => setTimeout(r, 0))
  return wrapper
}

beforeEach(() => {
  vi.mocked(toast.warning).mockClear()
})

describe('KnowledgeBase/文件选择过滤', () => {
  it('多选中夹带超过 50MB 的文件时被过滤，不进入批量上传队列', async () => {
    const wrapper = await mountView()
    const input = wrapper.find('input[type="file"]:not([webkitdirectory])').element as HTMLInputElement

    await selectFiles(input, [
      makeFile('ok-1.md'),
      makeFile('ok-2.pdf'),
      makeFile('huge.pdf', MAX_KNOWLEDGE_FILE_SIZE + 1)
    ])

    expect(toast.warning).toHaveBeenCalledWith(expect.stringContaining('已过滤 1 个'))
    const queued = (wrapper.vm as any).batchUpload.items.value.map((i: any) => i.file.name)
    expect(queued).toEqual(['ok-1.md', 'ok-2.pdf'])
    wrapper.unmount()
  })

  it('多选中夹带不支持格式的文件时同样被过滤', async () => {
    const wrapper = await mountView()
    const input = wrapper.find('input[type="file"]:not([webkitdirectory])').element as HTMLInputElement

    await selectFiles(input, [makeFile('a.md'), makeFile('b.exe'), makeFile('c.txt')])

    expect(toast.warning).toHaveBeenCalledWith(expect.stringContaining('已过滤 1 个'))
    const queued = (wrapper.vm as any).batchUpload.items.value.map((i: any) => i.file.name)
    expect(queued).toEqual(['a.md', 'c.txt'])
    wrapper.unmount()
  })

  it('单选一个超限文件时不打开上传对话框，直接提示过滤', async () => {
    const wrapper = await mountView()
    const input = wrapper.find('input[type="file"]:not([webkitdirectory])').element as HTMLInputElement

    await selectFiles(input, [makeFile('huge-single.pdf', MAX_KNOWLEDGE_FILE_SIZE + 1)])

    expect(toast.warning).toHaveBeenCalledWith(expect.stringContaining('已过滤 1 个'))
    expect((wrapper.vm as any).showUploadDialog).toBe(false)
    wrapper.unmount()
  })

  it('全部合规时不提示过滤，单文件仍走原单文件对话框流程', async () => {
    const wrapper = await mountView()
    const input = wrapper.find('input[type="file"]:not([webkitdirectory])').element as HTMLInputElement

    await selectFiles(input, [makeFile('single.md')])

    expect(toast.warning).not.toHaveBeenCalled()
    expect((wrapper.vm as any).showUploadDialog).toBe(true)
    expect((wrapper.vm as any).selectedFileName).toBe('single.md')
    wrapper.unmount()
  })
})
