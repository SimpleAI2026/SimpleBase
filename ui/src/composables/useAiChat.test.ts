import { toast } from 'vue-sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAiChat } from './useAiChat'

const chat = vi.fn()
const stream = vi.fn()

vi.mock('../services/api', () => ({
  api: {
    llm: {
      chat: (...args: unknown[]) => chat(...args),
      stream: (...args: unknown[]) => stream(...args)
    }
  }
}))

describe('useAiChat', () => {
  beforeEach(() => {
    chat.mockReset()
    stream.mockReset()
  })

  it('ignores empty input and non-stream chat success/error', async () => {
    const chatApi = useAiChat({
      projectId: () => 'p1',
      model: () => ' openai:default ',
      streaming: () => false
    })
    await chatApi.send('   ')
    expect(chat).not.toHaveBeenCalled()

    chat.mockResolvedValue({ content: 'hi', model: 'm', provider: 'p' })
    await chatApi.send('hello')
    expect(chatApi.messages.value.at(-1)?.content).toBe('hi')
    expect(chat.mock.calls[0][1].model).toBeUndefined()

    chat.mockRejectedValue(new Error('down'))
    await chatApi.send('again')
    expect(toast.error).toHaveBeenCalledWith('down')
    chatApi.clear()
    expect(chatApi.messages.value).toEqual([])
  })

  it('streams chunks, maps errors, and can stop', async () => {
    const close = vi.fn()
    stream.mockImplementation((_pid, _req, handlers) => {
      handlers.onChunk('A')
      handlers.onChunk('B')
      handlers.onEnd()
      return { close }
    })
    const chatApi = useAiChat({
      projectId: () => 'p1',
      model: () => 'gpt-4o-mini',
      streaming: () => true
    })
    await chatApi.send('stream me')
    expect(chatApi.messages.value.some((m) => m.content === 'AB')).toBe(true)

    stream.mockImplementation((_pid, _req, handlers) => {
      handlers.onError(new Error('sse'))
      return { close }
    })
    await chatApi.send('fail stream')
    expect(toast.error).toHaveBeenCalledWith('sse')
    chatApi.stop()
    expect(close).toHaveBeenCalled()
    chatApi.sending.value = true
    chatApi.clear()
    expect(chatApi.messages.value.length).toBeGreaterThan(0)
  })
})
