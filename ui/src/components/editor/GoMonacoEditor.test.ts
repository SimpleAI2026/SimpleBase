import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

const editor = {
  getValue: vi.fn(() => 'package main'),
  setValue: vi.fn(),
  updateOptions: vi.fn(),
  onDidChangeModelContent: vi.fn((cb: () => void) => {
    cb()
    return { dispose: vi.fn() }
  }),
  dispose: vi.fn()
}

vi.mock('monaco-editor/editor.js', () => ({
  languages: { register: vi.fn() },
  editor: {
    defineTheme: vi.fn(),
    create: vi.fn(() => editor)
  }
}))

vi.mock('monaco-editor/editor/editor.worker.js?worker', () => ({
  default: class EditorWorker {}
}))

import GoMonacoEditor from './GoMonacoEditor.vue'

describe('GoMonacoEditor', () => {
  it('bootstraps monaco, syncs values, and disposes', async () => {
    const w = mount(GoMonacoEditor, { props: { modelValue: 'package main', minHeight: 200 } })
    expect(editor.onDidChangeModelContent).toHaveBeenCalled()
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['package main'])
    editor.getValue.mockReturnValueOnce('package main')
    await w.setProps({ modelValue: 'package other' })
    expect(editor.setValue).toHaveBeenCalledWith('package other')
    await w.setProps({ readOnly: true })
    expect(editor.updateOptions).toHaveBeenCalledWith({ readOnly: true })
    w.unmount()
    expect(editor.dispose).toHaveBeenCalled()

    const env = (globalThis as unknown as { MonacoEnvironment?: { getWorker: () => unknown } }).MonacoEnvironment
    expect(env?.getWorker()).toBeTruthy()

    const w2 = mount(GoMonacoEditor, { props: { modelValue: 'x' } })
    w2.unmount()
  })
})
