<template>
  <PageContainer title="LLM 对话" subtitle="多供应商大模型对话测试（支持流式输出）">
    <a-card class="sb-card" title="会话配置">
      <div class="sb-toolbar">
        <a-input v-model:value="projectId" style="width: 220px" placeholder="项目 ID" />
        <a-input v-model:value="model" style="width: 220px" placeholder="模型（可选，默认供应商配置）" />
        <a-space :size="6">
          <span class="sb-label">供应商</span>
          <a-tag v-for="p in providers" :key="p" color="orange">{{ p }}</a-tag>
          <span v-if="!providers.length" class="sb-label">未配置</span>
        </a-space>
        <a-switch v-model:checked="streaming" checked-children="流式" un-checked-children="整段" />
        <a-button @click="loadProviders">
          <template #icon><ReloadOutlined /></template>
          刷新供应商
        </a-button>
        <a-button :disabled="sending || !messages.length" @click="clear">清空会话</a-button>
      </div>
    </a-card>

    <a-card class="sb-card" title="对话">
      <div class="sb-chat">
        <a-empty v-if="!messages.length" description="开始一段对话吧" />
        <div
          v-for="(m, i) in messages"
          :key="i"
          class="sb-msg"
          :class="m.role === 'user' ? 'sb-msg-user' : 'sb-msg-assistant'"
        >
          <span class="sb-avatar">
            <UserOutlined v-if="m.role === 'user'" />
            <RobotOutlined v-else />
          </span>
          <div class="sb-bubble">
            <pre>{{ m.content }}<span v-if="sending && streaming && i === messages.length - 1" class="sb-cursor">▍</span></pre>
          </div>
        </div>
      </div>
      <div class="sb-input-bar">
        <a-textarea
          v-model:value="input"
          :rows="2"
          placeholder="输入消息，Enter 发送，Shift+Enter 换行"
          @keydown.enter.exact.prevent="send"
        />
        <a-button v-if="sending" danger @click="stop">
          <template #icon><StopOutlined /></template>
          停止
        </a-button>
        <a-button v-else type="primary" :disabled="!input.trim()" @click="send">
          <template #icon><SendOutlined /></template>
          发送
        </a-button>
      </div>
    </a-card>
  </PageContainer>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  SendOutlined,
  StopOutlined,
  ReloadOutlined,
  RobotOutlined,
  UserOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { LlmMessage, LlmStreamConnection } from '../services/api'
import PageContainer from '../components/PageContainer.vue'

interface ChatMsg {
  role: 'user' | 'assistant'
  content: string
}

const projectId = ref('proj-01')
const providers = ref<string[]>([])
const model = ref('')
const streaming = ref(true)
const input = ref('')
const messages = ref<ChatMsg[]>([])
const sending = ref(false)
let conn: LlmStreamConnection | null = null

async function loadProviders() {
  if (!projectId.value.trim()) {
    message.warning('请先填写项目 ID')
    return
  }
  try {
    providers.value = await api.llm.providers(projectId.value.trim())
  } catch (e: any) {
    message.error(e?.message || '加载供应商失败')
  }
}

async function send() {
  const text = input.value.trim()
  if (!text || sending.value) return
  if (!projectId.value.trim()) {
    message.warning('请先填写项目 ID')
    return
  }
  input.value = ''
  messages.value.push({ role: 'user', content: text })
  const req = {
    model: model.value.trim() || undefined,
    messages: messages.value.map((m): LlmMessage => ({ role: m.role, content: m.content }))
  }
  sending.value = true
  if (streaming.value) {
    const reply: ChatMsg = { role: 'assistant', content: '' }
    messages.value.push(reply)
    conn = api.llm.stream(projectId.value.trim(), req, {
      onChunk: (t) => {
        reply.content += t
      },
      onEnd: () => {
        sending.value = false
        conn = null
      },
      onError: (e: any) => {
        sending.value = false
        conn = null
        message.error(e?.message || '流式请求失败')
      }
    })
    return
  }
  try {
    const resp = await api.llm.chat(projectId.value.trim(), req)
    messages.value.push({ role: 'assistant', content: resp.content })
  } catch (e: any) {
    message.error(e?.message || '请求失败')
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

onMounted(loadProviders)
</script>

<style scoped>
.sb-label {
  color: var(--sb-text-secondary);
  font-size: 13px;
}
.sb-chat {
  min-height: 240px;
  max-height: 480px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 0;
}
.sb-msg {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}
.sb-msg-user {
  flex-direction: row-reverse;
}
.sb-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--sb-bg-soft);
  border: 1px solid var(--sb-border-soft);
  color: var(--sb-text-secondary);
  flex-shrink: 0;
}
.sb-msg-user .sb-avatar {
  color: var(--sb-primary);
  border-color: var(--sb-primary);
}
.sb-bubble {
  max-width: 75%;
  padding: 8px 12px;
  border-radius: var(--sb-radius-sm);
  background: var(--sb-bg-soft);
  border: 1px solid var(--sb-border-soft);
}
@media (max-width: 768px) {
  .sb-bubble {
    max-width: 85%;
  }
  .sb-input-bar {
    flex-wrap: wrap;
  }
  .sb-input-bar .ant-input {
    flex: 1 1 100%;
  }
}
.sb-msg-user .sb-bubble {
  background: var(--sb-primary-light);
  border-color: transparent;
}
.sb-bubble pre {
  margin: 0;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--sb-text);
}
.sb-cursor {
  color: var(--sb-primary);
  animation: sb-blink 1s step-end infinite;
}
@keyframes sb-blink {
  50% {
    opacity: 0;
  }
}
.sb-input-bar {
  display: flex;
  gap: 8px;
  align-items: flex-end;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--sb-border-soft);
}
</style>
