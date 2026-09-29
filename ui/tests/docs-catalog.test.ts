import { describe, expect, it } from 'vitest'
import {
  buildCatalogFrom,
  defaultModuleId,
  defaultSlug,
  docCatalog,
  docsRelPath,
  firstH1,
  getModule,
  getPage,
  githubBlobUrl,
  loadedMarkdownCount,
  loadMarkdownRaw,
  parseFrontmatter,
  toPosix
} from './catalog'

describe('docs catalog', () => {
  it('builds modules from repo docs and meta', () => {
    expect(loadedMarkdownCount).toBeGreaterThan(0)
    expect(docCatalog.length).toBeGreaterThan(0)
    const id = defaultModuleId()
    expect(getModule(id)?.pages.length).toBeGreaterThan(0)
    const slug = defaultSlug(id)
    expect(getPage(id, slug)?.filePath).toBeTruthy()
    expect(defaultSlug('no-such-module')).toBe('index')
    expect(getModule('missing')).toBeUndefined()
    expect(getPage(id, 'definitely-missing')).toBeUndefined()
  })

  it('loads markdown bodies and builds github urls', () => {
    const page = getPage(defaultModuleId(), defaultSlug(defaultModuleId()))
    expect(page).toBeTruthy()
    const body = loadMarkdownRaw(page!.filePath)
    expect(body).toBeTruthy()
    expect(loadMarkdownRaw('does/not/exist.md')).toBeNull()
    expect(githubBlobUrl('ops/x.md')).toBe(
      'https://github.com/linkxzhou/SimpleBase/blob/main/docs/ops/x.md'
    )
  })

  it('parses frontmatter, glob keys, and leftover modules', () => {
    expect(parseFrontmatter('plain')).toEqual({ data: {}, body: 'plain' })
    expect(parseFrontmatter('---\nno-end').body).toContain('no-end')
    const quoted = parseFrontmatter('---\ntitle: "Hello"\nalt: \'World\'\norder: 3\nskipped\n---\n# Body')
    expect(quoted.data.title).toBe('Hello')
    expect(quoted.data.alt).toBe('World')
    expect(quoted.data.order).toBe(3)
    expect(quoted.body).toContain('# Body')
    expect(firstH1('# Title\n')).toBe('Title')
    expect(firstH1('no heading')).toBeUndefined()
    expect(toPosix('C\\\\docs\\\\ops\\\\x.md').includes('/')).toBe(true)
    expect(docsRelPath('C:\\repo\\docs\\ops\\x.md')).toBe('ops/x.md')
    expect(docsRelPath('@docs/sdk/index.md')).toBe('sdk/index.md')
    expect(docsRelPath('unrelated/file.md')).toBeNull()

    const leftover = buildCatalogFrom(
      {
        '/x/docs/unlisted/hello.md': '---\norder: 5\n---\n# Hello leftover',
        '/x/docs/unlisted/z.md': '---\ntitle: "Zed"\norder: 5\n---\nbody',
        '/x/docs/getting-started/index.md': '# Index page',
        '/x/docs/getting-started/orphan.md': 'no title here',
        '/x/docs/getting-started/aardvark.md': '---\norder: 1\n---\n# A',
        '/x/docs/getting-started/beta.md': '---\norder: 1\n---\n# B',
        '/x/docs/flat.md': '# ignored',
        '/x/docs/_hidden/x.md': '# hidden',
        '/x/docs/empty-mod/readme.txt': 'not markdown',
        '/x/docs/readme-mod/README.md': '# Readme as index',
        'no-match': '# x'
      },
      { modules: [{ id: 'getting-started', title: '入门' }] }
    )
    expect(leftover.some((m) => m.id === 'unlisted' && m.order === 1000)).toBe(true)
    const gs = leftover.find((m) => m.id === 'getting-started')
    expect(gs?.pages.some((p) => p.slug === 'orphan' && p.title === 'orphan')).toBe(true)
    expect(gs?.pages.find((p) => p.slug === 'index')?.order).toBe(0)
    expect(leftover.some((m) => m.id === 'readme-mod' && m.pages[0].slug === 'index')).toBe(true)
    expect(buildCatalogFrom({}, {}).length).toBe(0)
    const noIndex = docCatalog.find((m) => m.pages.length && !m.pages.some((p) => p.slug === 'index'))
    if (noIndex) expect(defaultSlug(noIndex.id)).toBe(noIndex.pages[0].slug)
  })
})
