import { errorMessage } from '@/utils/format'
import { ref } from 'vue'
import { toast } from 'vue-sonner'
import { api } from '../services/api'
import type { LlmMessage, LlmStreamConnection } from '../services/api'
import type { AgentToolCallCard } from '../services/types'

export interface ChatMsg {
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolCalls?: AgentToolCallCard[]
  error?: string
  canceled?: boolean
  thinking?: string
  /** 发出该条助手消息的 Agent module，用于头像与侧栏图标对齐。 */
  module?: string
}

export function useAiChat(opts: {
  projectId: () => string
  model: () => string | undefined
  streaming: () => boolean
}) {
  const messages = ref<ChatMsg[]>([])
  const sending = ref(false)
  let conn: LlmStreamConnection | null = null

  async function send(text: string) {
    const content = text.trim()
    if (!content || sending.value) return
    const reqMessages: LlmMessage[] = messages.value.map((m) => ({ role: m.role, content: m.content }))
    reqMessages.push({ role: 'user', content })
    if (!reqMessages.length) {
      toast.warning('消息不能为空')
      return
    }

    messages.value.push({ role: 'user', content })
    const rawModel = opts.model()?.trim() || undefined
    const model = rawModel?.includes(':default') ? undefined : rawModel
    const req = { model, messages: reqMessages }
    sending.value = true

    if (opts.streaming()) {
      const reply: ChatMsg = { role: 'assistant', content: '' }
      messages.value.push(reply)
      conn = api.llm.stream(opts.projectId(), req, {
        onChunk: (t) => {
          reply.content += t
        },
        onEnd: () => {
          sending.value = false
          conn = null
        },
        onError: (e) => {
          sending.value = false
          conn = null
          if (e) toast.error(errorMessage(e, '流式请求失败'))
        }
      })
      return
    }

    try {
      const resp = await api.llm.chat(opts.projectId(), req)
      messages.value.push({ role: 'assistant', content: resp.content })
    } catch (e) {
      toast.error(errorMessage(e, '请求失败'))
    } finally {
      sending.value = false
    }
  }

  function stop() {
    conn?.close()
    conn = null
    sending.value = false
  }

  function clear() {
    if (sending.value) return
    messages.value = []
  }

  return { messages, sending, send, stop, clear }
}
