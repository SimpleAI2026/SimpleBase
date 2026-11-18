/**
 * 把 Blob 存成本地文件（planv6.7 ui-simplify-ux-plan B8）。
 *
 * 原先 6 处各自 `a.click()` 后同步 `revokeObjectURL`：部分浏览器（Safari / 旧 Firefox）
 * 在下载真正开始前就失去了 URL，偶发下载到空文件。这里把锚点挂进文档、点击后移除，
 * 并在下一个宏任务再回收 URL。
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.rel = 'noopener'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

/** 取对象键 / 路径的最后一段作为下载文件名。 */
export function basename(path: string, fallback = 'download'): string {
  return path.split('/').filter(Boolean).pop() || fallback
}
