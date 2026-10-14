/** 对象存储文件预览：按扩展名判断可预览的类型（S3Object 没有 content-type）。 */

export type PreviewKind = 'image' | 'markdown' | 'text' | 'unsupported'

export interface PreviewInfo {
  kind: PreviewKind
  /** 小写扩展名，不含点；无扩展名为空串 */
  ext: string
  /** key 的最后一段，用作标题与下载文件名 */
  basename: string
}

/** 文本类预览的大小上限（字节），超过则只提供下载。 */
export const MAX_TEXT_PREVIEW_BYTES = 1024 * 1024

const IMAGE_EXTS = new Set(['jpg', 'jpeg', 'png', 'webp', 'svg', 'gif', 'bmp', 'ico', 'avif'])
const MARKDOWN_EXTS = new Set(['md', 'markdown'])
const TEXT_EXTS = new Set([
  'txt', 'log', 'logs', 'json', 'csv', 'yaml', 'yml', 'xml', 'ini', 'conf', 'sql'
])

export function getPreviewInfo(key: string): PreviewInfo {
  const basename = key.split('/').filter(Boolean).pop() || key
  const dot = basename.lastIndexOf('.')
  // 点开头文件（如 .env）与无扩展名文件都视为没有扩展名
  const ext = dot > 0 && dot < basename.length - 1 ? basename.slice(dot + 1).toLowerCase() : ''
  let kind: PreviewKind = 'unsupported'
  if (IMAGE_EXTS.has(ext)) kind = 'image'
  else if (MARKDOWN_EXTS.has(ext)) kind = 'markdown'
  else if (TEXT_EXTS.has(ext)) kind = 'text'
  return { kind, ext, basename }
}

export function isTextLike(kind: PreviewKind): boolean {
  return kind === 'markdown' || kind === 'text'
}
