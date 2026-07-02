import { describe, it, expect } from 'vitest'
import { escapeHtml, isSafeUrl } from '@/utils/security'
import { formatFileSize } from '@/utils/index'

describe('security/escapeHtml', () => {
  it('转义所有 HTML 特殊字符', () => {
    expect(escapeHtml('<script>alert("x")</script>')).toBe(
      '&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;',
    )
  })

  it('转义单引号与 &', () => {
    expect(escapeHtml(`Tom & Jerry's`)).toBe('Tom &amp; Jerry&#039;s')
  })

  it('无特殊字符时原样返回', () => {
    expect(escapeHtml('hello world')).toBe('hello world')
  })
})

describe('security/isSafeUrl', () => {
  it('接受 http / https', () => {
    expect(isSafeUrl('http://example.com')).toBe(true)
    expect(isSafeUrl('https://example.com/path?q=1')).toBe(true)
  })

  it('拒绝 javascript / data 等协议', () => {
    expect(isSafeUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeUrl('data:text/html,<script>')).toBe(false)
  })

  it('拒绝非法 URL', () => {
    expect(isSafeUrl('not a url')).toBe(false)
  })
})

describe('utils/formatFileSize', () => {
  it('0 字节', () => {
    expect(formatFileSize(0)).toBe('0 B')
  })

  it('按单位换算', () => {
    expect(formatFileSize(1024)).toBe('1.00 KB')
    expect(formatFileSize(1024 * 1024)).toBe('1.00 MB')
    expect(formatFileSize(1536)).toBe('1.50 KB')
  })
})
