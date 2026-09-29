import { describe, expect, it, vi } from 'vitest'
import { renderMarkdown } from './render'

describe('renderMarkdown', () => {
  it('rewrites relative doc links and sanitizes HTML', () => {
    const html = renderMarkdown(
      '[same](./overview.md)\n[index](./index.md)\n[up](../ops/deploy.md)\n[abs](/docs/x/y)\n[ext](https://example.com)\n[hash](#sec)\n[mail](mailto:a@b.c)',
      'gofunction'
    )
    expect(html).toContain('/docs/gofunction/overview')
    expect(html).toContain('/docs/gofunction')
    expect(html).toContain('/docs/ops/deploy')
    expect(html).toContain('/docs/x/y')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('noopener')
  })

  it('never throws and escapes render failures', async () => {
    expect(() => renderMarkdown('', 'x')).not.toThrow()
    expect(() => renderMarkdown('<script>alert(1)</script>', 'x')).not.toThrow()
    expect(renderMarkdown('[empty]()', 'gofunction')).toBeTruthy()

    const { marked } = await import('marked')
    const spy = vi.spyOn(marked, 'parse').mockImplementation(() => {
      throw new Error('boom <tag> &')
    })
    const html = renderMarkdown('x', 'gofunction')
    expect(html).toContain('文档渲染失败')
    expect(html).toContain('&lt;tag&gt;')
    spy.mockRestore()

    const spy2 = vi.spyOn(marked, 'parse').mockImplementation(() => {
      throw 'raw'
    })
    expect(renderMarkdown('x', 'gofunction')).toContain('raw')
    spy2.mockRestore()
  })
})
