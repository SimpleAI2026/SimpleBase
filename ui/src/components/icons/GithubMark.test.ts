import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import GithubMark from './GithubMark.vue'

describe('GithubMark', () => {
  it('renders an svg mark', () => {
    const w = mount(GithubMark)
    expect(w.find('svg').exists()).toBe(true)
  })
})
