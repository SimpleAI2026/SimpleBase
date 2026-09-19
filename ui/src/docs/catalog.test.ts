import { describe, expect, it } from 'vitest'
import {
  defaultModuleId,
  defaultSlug,
  docCatalog,
  getModule,
  getPage,
  githubBlobUrl,
  loadedMarkdownCount,
  loadMarkdownRaw
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
})
