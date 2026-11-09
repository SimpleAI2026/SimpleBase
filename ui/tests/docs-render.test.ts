import { describe, expect, it, vi } from 'vitest'
import { renderMarkdown, rewriteDocHref } from '@/docs/render'

describe('renderMarkdown', () => {
  it('rewrites relative doc links and sanitizes HTML', () => {
    const { html } = renderMarkdown(
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

  it('builds stable anchors, code blocks, table wrappers and callouts safely', () => {
    const source = '# Title\n## Hé **World**\n## Hé **World**\n### Database {#docs:db:section}\n#### Detail\n> **Warning** be careful\n\n| A | B |\n|---|---|\n|x|y|\n\n```sql\nSELECT 1 < 2;\n```'
    const { html, toc } = renderMarkdown(source, 'database')
    expect(toc).toEqual([
      { id: 'hé-world', text: 'Hé World', level: 2 },
      { id: 'hé-world-2', text: 'Hé World', level: 2 },
      { id: 'docs:db:section', text: 'Database', level: 3 }
    ])
    expect(html).toContain('id="docs:db:section"')
    expect(html).not.toContain('{#docs:db:section}')
    expect(html).toContain('data-lang="sql"')
    expect(html).toContain('data-copy')
    expect(html).toContain('SELECT 1 &lt; 2;')
    expect(html).toContain('docs-table-scroll')
    expect(html).toContain('data-callout="warning"')
    expect(renderMarkdown('<script>alert(1)</script>', 'database').html).not.toContain('<script>')
    expect(rewriteDocHref('../../examples/README.md', 'sdk')).toBe('https://github.com/linkxzhou/SimpleBase/blob/main/examples/README.md')
    expect(rewriteDocHref('../ops/deployment.md#deployment', 'sdk')).toBe('/docs/ops/deployment#deployment')
  })

  it('never throws and escapes render failures', async () => {
    expect(() => renderMarkdown('', 'x')).not.toThrow()
    expect(() => renderMarkdown('<script>alert(1)</script>', 'x')).not.toThrow()
    expect(renderMarkdown('[empty]()', 'gofunction').html).toBeTruthy()

    const { marked } = await import('marked')
    const spy = vi.spyOn(marked, 'parse').mockImplementation(() => {
      throw new Error('boom <tag> &')
    })
    const { html } = renderMarkdown('x', 'gofunction')
    expect(html).toContain('文档渲染失败')
    expect(html).toContain('&lt;tag&gt;')
    spy.mockRestore()

    const spy2 = vi.spyOn(marked, 'parse').mockImplementation(() => {
      throw 'raw'
    })
    expect(renderMarkdown('x', 'gofunction').html).toContain('raw')
    spy2.mockRestore()
  })
})
