import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'

// ========================================
// useMarkdown —— AI/Markdown 内容渲染的唯一入口
// ========================================
//
// 单一事实源：所有把模型输出/Markdown 渲染成 HTML 再 v-html 的地方都必须走这里。
// 渲染链路：markdown-it（html:false，禁用原始 HTML）→ DOMPurify.sanitize（纵深防御，
// 拦截任何仍可能逃逸的脚本/事件属性），杜绝 AI 内容里的 XSS。

export function useMarkdown() {
  const md = new MarkdownIt({ html: false, linkify: true, typographer: true })

  function renderMarkdown(content: string): string {
    return DOMPurify.sanitize(md.render(content || ''))
  }

  return { renderMarkdown }
}
