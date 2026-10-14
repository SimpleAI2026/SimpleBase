import { describe, expect, it } from 'vitest'
import { MAX_TEXT_PREVIEW_BYTES, getPreviewInfo, isTextLike } from '@/utils/filePreview'

describe('getPreviewInfo', () => {
  it('detects image types case-insensitively', () => {
    for (const name of ['a.jpg', 'a.JPEG', 'a.png', 'a.webp', 'a.svg', 'dir/sub/PIC.PNG']) {
      expect(getPreviewInfo(name).kind, name).toBe('image')
    }
  })

  it('detects markdown and text types', () => {
    expect(getPreviewInfo('README.md').kind).toBe('markdown')
    expect(getPreviewInfo('doc.markdown').kind).toBe('markdown')
    for (const name of ['a.txt', 'app.log', 'app.LOGS', 'cfg.yaml', 'x.json', 'q.sql']) {
      expect(getPreviewInfo(name).kind, name).toBe('text')
    }
  })

  it('treats unknown, extension-less and dot-files as unsupported', () => {
    expect(getPreviewInfo('archive.zip').kind).toBe('unsupported')
    expect(getPreviewInfo('Makefile').kind).toBe('unsupported')
    expect(getPreviewInfo('.env').kind).toBe('unsupported')
    expect(getPreviewInfo('trailing.').kind).toBe('unsupported')
    expect(getPreviewInfo('a.tar.gz').ext).toBe('gz')
  })

  it('extracts basename and ext from nested keys', () => {
    const info = getPreviewInfo('reports/2026/q1.TXT')
    expect(info).toEqual({ kind: 'text', ext: 'txt', basename: 'q1.TXT' })
    expect(getPreviewInfo('').basename).toBe('')
  })

  it('exposes text-like helper and size limit', () => {
    expect(isTextLike('text')).toBe(true)
    expect(isTextLike('markdown')).toBe(true)
    expect(isTextLike('image')).toBe(false)
    expect(isTextLike('unsupported')).toBe(false)
    expect(MAX_TEXT_PREVIEW_BYTES).toBe(1024 * 1024)
  })
})
