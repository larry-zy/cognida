/**
 * 计算文件内容的 SHA-256 十六进制哈希，用于上传前判重（须与后端算法一致）。
 *
 * 注意：这里刻意保留 arrayBuffer() 一次性读入 + crypto.subtle.digest 的口径——
 * Web Crypto 的 digest() 不支持增量/流式摘要，改用分块流式必须引入 userland 增量
 * SHA-256 实现（新依赖），且一旦算法/口径与后端不一致就会破坏上传去重逻辑，风险高，
 * 故不改哈希计算方式。大文件内存峰值问题留待后端/依赖层面统一处理。
 */
export async function computeFileHash(file: File): Promise<string> {
  const buffer = await file.arrayBuffer()
  const digest = await crypto.subtle.digest('SHA-256', buffer)
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}
