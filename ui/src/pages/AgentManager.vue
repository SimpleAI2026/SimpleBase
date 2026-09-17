<template>
  <ProjectScope>
    <PageContainer title="Cloud Agent" subtitle="按模块的只读 Agent；composer 输入 @ 点名">
      <div class="ca-layout">
        <a-card class="sb-card ca-list" title="Agents">
          <div class="sb-toolbar" style="margin-bottom: 12px">
            <a-button :loading="loading" @click="loadAgents">
              <template #icon><ReloadOutlined /></template>
              刷新
            </a-button>
            <a-button type="primary" @click="openCreate">
              <template #icon><PlusOutlined /></template>
              新建
            </a-button>
          </div>
          <SbEmptyState v-if="!loading && !agents.length" description="还没有 Agent" action-text="创建" @action="openCreate" />
          <button
            v-for="a in agents"
            :key="a.id"
            type="button"
            class="ca-item"
            :class="{ 'is-active': a.id === activeId }"
            @click="activeId = a.id"
          >
            <div class="ca-item-top">
              <strong>{{ a.name }}</strong>
              <a-tag>{{ a.module }}</a-tag>
            </div>
            <div class="ca-item-desc">{{ a.description || '无描述' }}</div>
            <div class="ca-item-ops">
              <a-button type="link" size="small" @click.stop="openEdit(a)">编辑</a-button>
              <a-button type="link" size="small" danger @click.stop="removeAgent(a)">删除</a-button>
            </div>
          </button>
        </a-card>

        <a-card class="sb-card ca-chat" :title="activeAgent ? '@' + activeAgent.name : '对话'">
          <AiChat
            :project-id="project.id"
            :show-toolbar="true"
            :mention-agents="mentionAgents"
            :messages="chatMessages"
            :sending="sending"
            :custom-send="onSend"
            :streaming="true"
            :placeholder="composerPlaceholder"
            @stop="onStop"
          >
            <template #toolbar>
              <a-button :disabled="sending || !chatMessages.length" @click="resetThread">新会话</a-button>
              <span class="ca-hint">点名 {{ activeAgent ? '@' + activeAgent.name : '一个 Agent' }} 后发送；工具只读</span>
            </template>
            <template #empty>
              <SbEmptyState v-if="!chatMessages.length" description="用 @ 点名左侧 Agent，询问数据库、对象或日志" />
            </template>
          </AiChat>
        </a-card>
      </div>

      <SbModal :open="modalOpen" :title="editing ? '编辑 Agent' : '新建 Agent'" :confirm-loading="saving" @ok="saveAgent" @update:open="(v: boolean) => (modalOpen = v)">
        <a-form layout="vertical">
          <a-form-item label="名称" required>
            <a-input v-model:value="form.name" placeholder="Database" />
          </a-form-item>
          <a-form-item label="模块">
            <a-select v-model:value="form.module" :options="moduleOptions" @change="onModuleChange" />
          </a-form-item>
          <a-form-item label="描述">
            <a-input v-model:value="form.description" />
          </a-form-item>
          <a-form-item label="System prompt">
            <a-textarea v-model:value="form.system_prompt" :rows="4" placeholder="可选；叠在模块模板之上" />
          </a-form-item>
          <a-form-item label="只读工具">
            <a-select
              v-model:value="form.tool_ids"
              mode="multiple"
              :options="toolOptions"
              placeholder="选择只读工具"
            />
          </a-form-item>
        </a-form>
      </SbModal>
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { AgentModuleInfo, CloudAgent, LlmStreamConnection } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import SbModal from '../components/modal/SbModal.vue'
import AiChat from '../components/ai/AiChat.vue'
import type { ChatMsg } from '../composables/useAiChat'

const project = useProjectStore()
const loading = ref(false)
const saving = ref(false)
const agents = ref<CloudAgent[]>([])
const modules = ref<AgentModuleInfo[]>([])
const activeId = ref('')
const modalOpen = ref(false)
const editing = ref<CloudAgent | null>(null)
const form = ref({ name: '', module: 'database', description: '', system_prompt: '', tool_ids: [] as string[] })

const threadId = ref('')
const currentRunId = ref('')
const chatMessages = ref<ChatMsg[]>([])
const sending = ref(false)
let conn: LlmStreamConnection | null = null

const activeAgent = computed(() => agents.value.find((a) => a.id === activeId.value))
const mentionAgents = computed(() => agents.value.map((a) => ({ id: a.id, name: a.name, module: a.module })))
const composerPlaceholder = computed(() =>
  activeAgent.value
    ? `询问 @${activeAgent.value.name}，Enter 发送；Shift+Enter 换行`
    : '输入 @ 点名 Agent'
)

const moduleOptions = computed(() =>
  modules.value.map((m) => ({ label: `${m.name}（${m.id}）`, value: m.id }))
)
const toolOptions = computed(() => {
  const m = modules.value.find((x) => x.id === form.value.module)
  const ids = m?.default_tools?.length ? m.default_tools : ['list_databases', 'list_collections', 'readonly_sql', 'list_objects', 'head_object', 'search_logs', 'log_level_stats']
  return ids.map((id) => ({ label: id, value: id }))
})

watch(
  () => project.id,
  () => {
    void bootstrap()
  }
)

async function bootstrap() {
  await loadModules()
  await loadAgents()
  await ensureThread()
}

async function loadModules() {
  try {
    modules.value = await api.agents.modules(project.id)
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载模块失败')
  }
}

async function loadAgents() {
  loading.value = true
  try {
    agents.value = await api.agents.list(project.id)
    if (!activeId.value && agents.value.length) activeId.value = agents.value[0].id
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载 Agent 失败')
  } finally {
    loading.value = false
  }
}

async function ensureThread() {
  try {
    const list = await api.agentThreads.list(project.id)
    if (list.length) {
      threadId.value = list[0].id
    } else {
      const th = await api.agentThreads.create(project.id, 'Cloud Agent')
      threadId.value = th.id
    }
    const msgs = await api.agentThreads.messages(project.id, threadId.value)
    chatMessages.value = msgs.map((m) => ({
      role: m.role === 'assistant' ? 'assistant' : 'user',
      content: m.content,
      toolCalls: m.tool_calls
    }))
  } catch (e) {
    message.error(e instanceof Error ? e.message : '加载会话失败')
  }
}

function openCreate() {
  editing.value = null
  const first = modules.value[0]
  form.value = {
    name: first?.name || '',
    module: first?.id || 'database',
    description: first?.description || '',
    system_prompt: '',
    tool_ids: [...(first?.default_tools || [])]
  }
  modalOpen.value = true
}

function openEdit(a: CloudAgent) {
  editing.value = a
  form.value = {
    name: a.name,
    module: a.module,
    description: a.description,
    system_prompt: a.system_prompt,
    tool_ids: [...(a.tool_ids || [])]
  }
  modalOpen.value = true
}

function onModuleChange(mod: string) {
  const m = modules.value.find((x) => x.id === mod)
  if (m && !editing.value) {
    form.value.tool_ids = [...(m.default_tools || [])]
    if (!form.value.name) form.value.name = m.name
  }
}

async function saveAgent() {
  if (!form.value.name.trim()) {
    message.warning('请填写名称')
    return
  }
  saving.value = true
  try {
    if (editing.value) {
      await api.agents.patch(project.id, editing.value.id, form.value)
    } else {
      const created = await api.agents.create(project.id, form.value)
      activeId.value = created.id
    }
    modalOpen.value = false
    await loadAgents()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

async function removeAgent(a: CloudAgent) {
  try {
    await api.agents.remove(project.id, a.id)
    if (activeId.value === a.id) activeId.value = ''
    await loadAgents()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function resetThread() {
  if (sending.value) return
  try {
    const th = await api.agentThreads.create(project.id, 'Cloud Agent')
    threadId.value = th.id
    chatMessages.value = []
  } catch (e) {
    message.error(e instanceof Error ? e.message : '新建会话失败')
  }
}

async function onSend(text: string, mentions: { agent_id: string }[]) {
  const content = text.trim()
  if (!content || sending.value) return
  if (!threadId.value) await ensureThread()
  let used = mentions
  if (!used.length && activeAgent.value) {
    used = [{ agent_id: activeAgent.value.id }]
  }
  if (!used.length) {
    message.warning('请先选择或 @ 一个 Agent')
    return
  }
  chatMessages.value.push({ role: 'user', content })
  const reply: ChatMsg = { role: 'assistant', content: '', toolCalls: [] }
  chatMessages.value.push(reply)
  sending.value = true
  currentRunId.value = ''
  conn = api.agentThreads.streamRun(
    project.id,
    threadId.value,
    { content, mentions: used, stream: true },
    {
      onRun: (id) => {
        currentRunId.value = id
      },
      onToken: (t) => {
        reply.content += t
      },
      onToolCall: (name, args) => {
        reply.toolCalls = [...(reply.toolCalls || []), { name, arguments: args }]
      },
      onToolResult: (name, body) => {
        const cards = reply.toolCalls || []
        const last = [...cards].reverse().find((c) => c.name === name && !c.content)
        if (last) last.content = body
        else cards.push({ name, content: body })
        reply.toolCalls = [...cards]
      },
      onEnd: () => {
        sending.value = false
        conn = null
      },
      onError: (e) => {
        sending.value = false
        conn = null
        if (e) message.error(e instanceof Error ? e.message : '运行失败')
      }
    }
  )
}

function onStop() {
  conn?.close()
  conn = null
  sending.value = false
  if (currentRunId.value) {
    void api.agentThreads.cancel(project.id, currentRunId.value).catch(() => undefined)
  }
}

onMounted(() => {
  void bootstrap()
})
</script>

<style scoped>
.ca-layout {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: var(--sb-space-4);
  align-items: stretch;
}
.ca-list :deep(.ant-card-body) {
  padding: 16px;
}
.ca-item {
  display: block;
  width: 100%;
  text-align: left;
  border: 1px solid var(--sb-border-soft);
  background: var(--sb-bg-soft);
  border-radius: var(--sb-radius-sm);
  padding: 10px 12px;
  margin-bottom: 8px;
  cursor: pointer;
  color: inherit;
}
.ca-item.is-active {
  border-color: var(--sb-primary);
  background: var(--sb-primary-light);
}
.ca-item-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.ca-item-desc {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-xs);
  margin-top: 4px;
}
.ca-item-ops {
  display: flex;
  gap: 0;
  margin-top: 4px;
}
.ca-hint {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
}
@media (max-width: 768px) {
  .ca-layout {
    grid-template-columns: 1fr;
  }
}
</style>
