<template>
  <ProjectScope>
    <PageContainer subtitle="按模块的 Agent；composer 输入 @ 点名；工具默认只读，Sandbox 在云端隔离环境执行">
      <div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
        <!-- 左列：Agent 简化卡片（planv4.1 §3.2）。 -->
        <Card>
          <CardHeader class="border-b">
            <CardTitle>Agents</CardTitle>
            <CardAction>
              <div class="flex gap-2">
                <Button variant="outline" size="sm" :disabled="loading" @click="loadAgents">
                  <Spinner v-if="loading" data-icon="inline-start" />
                  <RefreshCwIcon v-else data-icon="inline-start" />
                  刷新
                </Button>
                <Button size="sm" @click="openCreate">
                  <PlusIcon data-icon="inline-start" />
                  新建
                </Button>
              </div>
            </CardAction>
          </CardHeader>
          <CardContent class="p-3">
            <SbEmptyState v-if="!loading && !agents.length" :icon="BotIcon" description="还没有 Agent" action-text="创建" @action="openCreate" />
            <div class="flex flex-col gap-2.5">
              <AgentCard
                v-for="a in agents"
                :key="a.id"
                :agent="a"
                :active="a.id === activeId"
                :schedule-summary="scheduleByAgent[a.id] ? scheduleSummary(scheduleByAgent[a.id]) : undefined"
                :schedule-enabled="!!scheduleByAgent[a.id]?.enabled"
                @select="activeId = a.id"
                @edit="openEdit(a)"
                @schedule="openSchedule(a)"
                @remove="removeAgent(a)"
              />
            </div>
          </CardContent>
        </Card>

        <!-- 右列：对话区（头部含会话切换器，planv4.1 §3.2）。 -->
        <Card class="flex min-h-[560px] flex-col">
          <CardHeader class="border-b">
            <CardTitle class="flex w-full items-center gap-3">
              <span class="shrink-0">{{ activeAgent ? '@' + activeAgent.name : '对话' }}</span>
              <ThreadSwitcher
                :threads="threads"
                :active-id="threadId"
                :busy="sending"
                @create="resetThread"
                @select="selectThread"
                @remove="removeThread"
              />
            </CardTitle>
          </CardHeader>
          <CardContent class="flex min-w-0 flex-1 flex-col gap-3 p-4">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <p v-if="statusText" role="status" class="text-xs text-muted-foreground">{{ statusText }}</p>
              <p v-else class="min-w-0 truncate text-xs text-muted-foreground">{{ toolsHint }}</p>
              <router-link v-if="activeAgentHasSandboxTools" :to="{ name: 'sandboxes' }" class="shrink-0 text-xs text-primary hover:underline">在云沙盒页查看</router-link>
            </div>
            <ConversationView
              :messages="chatMessages"
              :sending="sending"
              :can-retry="canRetry"
              @retry="retryLast"
            >
              <template #empty>
                <SbEmptyState v-if="!chatMessages.length" :icon="BotIcon" description="用 @ 点名左侧 Agent，询问数据库、对象或日志；Sandbox Agent 可在云端环境运行代码" />
              </template>
            </ConversationView>
            <AgentComposer
              v-model="draft"
              :sending="sending"
              :disabled="!threadId"
              :placeholder="composerPlaceholder"
              :mention-agents="mentionAgents"
              @send="onSend"
              @stop="onStop"
            />
          </CardContent>
        </Card>
      </div>

      <SbModal :open="modalOpen" :title="editing ? '编辑 Agent' : '新建 Agent'" :confirm-loading="saving" @ok="saveAgent" @update:open="(v: boolean) => (modalOpen = v)">
        <FieldGroup>
          <Field>
            <FieldLabel for="agent-name">名称</FieldLabel>
            <Input id="agent-name" v-model="form.name" placeholder="Database" />
          </Field>
          <Field>
            <FieldLabel>模块</FieldLabel>
            <Select :model-value="form.module" @update:model-value="onModuleChange">
              <SelectTrigger class="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem
                    v-for="opt in moduleOptions"
                    :key="opt.value"
                    :value="opt.value"
                    :disabled="opt.disabled"
                  >
                    {{ opt.label }}{{ opt.disabled ? '（未配置云沙盒）' : '' }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel for="agent-model">模型</FieldLabel>
            <Input id="agent-model" v-model="form.model_override" :placeholder="defaultModel ? `使用项目默认（${defaultModel}）` : '使用项目默认模型'" list="agent-model-options" />
            <datalist id="agent-model-options"><option v-for="model in modelOptions" :key="model" :value="model" /></datalist>
          </Field>
          <Field>
            <FieldLabel for="agent-desc">描述</FieldLabel>
            <Input id="agent-desc" v-model="form.description" />
          </Field>
          <Field>
            <FieldLabel for="agent-prompt">System prompt</FieldLabel>
            <Textarea id="agent-prompt" v-model="form.system_prompt" :rows="4" placeholder="可选；叠在模块模板之上" />
          </Field>
          <Field>
            <FieldLabel>工具</FieldLabel>
            <div class="flex flex-wrap gap-2">
              <Badge
                v-for="id in toolOptions"
                :key="id.value"
                as="button"
                type="button"
                :aria-pressed="form.tool_ids.includes(id.value)"
                :title="`工具：${id.label}`"
                :variant="form.tool_ids.includes(id.value) ? 'default' : 'outline'"
                class="h-auto min-h-8 cursor-pointer"
                @click="toggleTool(id.value)"
              >
                {{ id.label }}
              </Badge>
            </div>
          </Field>
        </FieldGroup>
      </SbModal>

      <AgentScheduleModal
        :open="scheduleModalOpen"
        :agent="scheduleAgent"
        :schedule="scheduleAgent ? (scheduleByAgent[scheduleAgent.id] || null) : null"
        :project-id="project.id"
        @update:open="(v: boolean) => (scheduleModalOpen = v)"
        @saved="onScheduleSaved"
        @removed="onScheduleRemoved"
        @view-thread="onViewScheduleThread"
      />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { BotIcon, PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { api } from '../services/api'
import type { AgentModuleInfo, AgentThread, CloudAgent } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import SbModal from '../components/modal/SbModal.vue'
import AgentCard from '../components/agent/AgentCard.vue'
import ThreadSwitcher from '../components/agent/ThreadSwitcher.vue'
import ConversationView from '../components/agent/ConversationView.vue'
import AgentComposer from '../components/agent/AgentComposer.vue'
import AgentScheduleModal from '../components/ai/AgentScheduleModal.vue'
import { useAgentConversation } from '../composables/useAgentConversation'
import type { ChatMsg } from '../composables/useAiChat'
import type { AgentSchedule } from '../services/types'

const project = useProjectStore()
const route = useRoute()
const router = useRouter()
const loading = ref(false)
const saving = ref(false)
const agents = ref<CloudAgent[]>([])
const modules = ref<AgentModuleInfo[]>([])
const activeId = ref('')
const modalOpen = ref(false)
const editing = ref<CloudAgent | null>(null)
const form = ref({ name: '', module: 'database', description: '', system_prompt: '', model_override: '', tool_ids: [] as string[] })
const defaultModel = ref('')
const modelOptions = ref<string[]>([])
const threads = ref<AgentThread[]>([])
const nextCursor = ref('')
const threadId = ref('')
const chatMessages = ref<ChatMsg[]>([])
const draft = ref('')

const scheduleByAgent = ref<Record<string, AgentSchedule>>({})
const scheduleModalOpen = ref(false)
const scheduleAgent = ref<CloudAgent | null>(null)

/** 会话状态层（planv4.1 BUG-01/03/09/10）。 */
const conv = useAgentConversation({
  projectId: () => project.id,
  threadId: () => threadId.value,
  messages: () => chatMessages.value
})
const sending = conv.sending
const statusText = conv.statusText
const failedRunId = conv.failedRunId

const activeAgent = computed(() => agents.value.find((a) => a.id === activeId.value))
const mentionAgents = computed(() => agents.value.map((a) => ({ id: a.id, name: a.name, module: a.module })))
const composerPlaceholder = computed(() =>
  activeAgent.value
    ? `询问 @${activeAgent.value.name}，Enter 发送；Shift+Enter 换行`
    : '输入 @ 点名 Agent'
)

const moduleOptions = computed(() =>
  modules.value.map((m) => ({
    label: `${m.name}（${m.id}）`,
    value: m.id,
    disabled: m.id === 'sandbox' && m.sandbox_available !== true
  }))
)

// 当前选中 Agent 是否含沙盒工具；决定 composer 旁的提示文案。
const activeAgentHasSandboxTools = computed(() => {
  const a = activeAgent.value
  if (!a) return false
  return (a.tool_ids || []).some((id) => id.startsWith('sandbox_'))
})

const toolsHint = computed(() => {
  const who = `点名 ${activeAgent.value ? '@' + activeAgent.value.name : '一个 Agent'} 后发送`
  if (activeAgentHasSandboxTools.value) {
    return `${who}；沙盒命令在云端隔离环境执行`
  }
  return `${who}；工具只读`
})
const toolOptions = computed(() => {
  const m = modules.value.find((x) => x.id === form.value.module)
  const ids = m?.default_tools?.length ? m.default_tools : ['list_databases', 'list_collections', 'readonly_sql', 'list_objects', 'head_object', 'search_logs', 'log_level_stats']
  return ids.map((id) => ({ label: id, value: id }))
})

/** 最后一条消息失败且记录了失败 run id 时可重试（BUG-03：走后端 retry_of_run_id）。 */
const canRetry = computed(() => {
  if (sending.value || !failedRunId.value) return false
  const last = chatMessages.value[chatMessages.value.length - 1]
  return last?.role === 'assistant' && !!last.error
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
  await loadSchedules()
  try {
    // BUG-07：优先用 /agents/models 提供模型候选。
    const res = await api.agents.models(project.id)
    defaultModel.value = res.default_model || ''
    modelOptions.value = res.models.map((m) => m.name)
    if (!modelOptions.value.length && defaultModel.value) modelOptions.value = [defaultModel.value]
  } catch { defaultModel.value = '' }
}

async function loadSchedules() {
  try {
    const list = await api.agentSchedules.list(project.id)
    const map: Record<string, AgentSchedule> = {}
    for (const s of list) map[s.agent_id] = s
    scheduleByAgent.value = map
  } catch (e) {
    toast.error(errorMessage(e, '加载定时任务失败'))
  }
}

function scheduleSummary(s: AgentSchedule): string {
  const presets: Record<string, string> = {
    '*/15 * * * *': '每 15 分钟',
    '0 * * * *': '每小时',
    '0 8 * * *': '每天 08:00 (UTC)',
    '0 8 * * 1': '每周一 08:00 (UTC)'
  }
  const cron = presets[s.cron_expr] || s.cron_expr
  return `${cron} ${s.enabled ? '已启用' : '已停用'}`
}

function openSchedule(a: CloudAgent) {
  scheduleAgent.value = a
  scheduleModalOpen.value = true
}

function onScheduleSaved(s: AgentSchedule) {
  scheduleByAgent.value = { ...scheduleByAgent.value, [s.agent_id]: s }
}

function onScheduleRemoved(scheduleId: string) {
  const map = { ...scheduleByAgent.value }
  for (const [agentId, s] of Object.entries(map)) {
    if (s.id === scheduleId) delete map[agentId]
  }
  scheduleByAgent.value = map
}

async function onViewScheduleThread(threadIdToView: string) {
  scheduleModalOpen.value = false
  await selectThread(threadIdToView)
}

async function loadThreadMessages(id: string) {
  const msgs = await api.agentThreads.messages(project.id, id)
  if (threadId.value !== id) return
  chatMessages.value = msgs.map((m) => ({
    role: m.role === 'assistant' ? 'assistant' : 'user', content: m.content,
    toolCalls: m.tool_calls,
    // BUG-06：刷新后按 run 状态还原失败态。
    error: m.error_code ? errorMessage({ code: m.error_code } as Error & { code?: string }, '运行失败') : undefined,
    canceled: m.run_status === 'canceled'
  }))
}

async function selectThread(id: string) {
  if (!id) return
  onStop()
  threadId.value = id
  chatMessages.value = []
  if (router?.replace) void router.replace({ query: { ...route.query, thread: id } })
  try { await loadThreadMessages(id) }
  catch (e) { toast.error(errorMessage(e, '加载会话失败')) }
}

async function renameThread(id: string, title: string) {
  try {
    const renamed = await api.agentThreads.rename(project.id, id, title)
    threads.value = threads.value.map((th) => th.id === id ? renamed : th)
  } catch (e) { toast.error(errorMessage(e, '重命名失败')) }
}

async function removeThread(id: string) {
  onStop()
  try {
    await api.agentThreads.remove(project.id, id)
    threads.value = threads.value.filter((th) => th.id !== id)
    if (threadId.value === id) {
      threadId.value = ''
      if (threads.value.length) await selectThread(threads.value[0].id)
      else await resetThread()
    }
  } catch (e) { toast.error(errorMessage(e, '删除会话失败')) }
}

async function loadModules() {
  try {
    modules.value = await api.agents.modules(project.id)
  } catch (e) {
    toast.error(errorMessage(e, '加载模块失败'))
  }
}

async function loadAgents() {
  loading.value = true
  try {
    agents.value = await api.agents.list(project.id)
    // BUG-11：通用助手默认选中。
    if (!activeId.value && agents.value.length) {
      const general = agents.value.find((a) => a.builtin_key === 'general')
      activeId.value = general?.id || agents.value[0].id
    }
  } catch (e) {
    toast.error(errorMessage(e, '加载 Agent 失败'))
  } finally {
    loading.value = false
  }
}

async function ensureThread() {
  try {
    const page = await api.agentThreads.page(project.id).catch(async () => ({ threads: await api.agentThreads.list(project.id), next_cursor: '' }))
    threads.value = page.threads
    nextCursor.value = page.next_cursor
    const queryId = typeof route.query.thread === 'string' ? route.query.thread : ''
    if (threads.value.length) {
      const selected = threads.value.find((th) => th.id === queryId)
      threadId.value = selected?.id || threads.value[0].id
    } else {
      const th = await api.agentThreads.create(project.id, '云 Agent')
      threads.value = [th]
      threadId.value = th.id
    }
    await loadThreadMessages(threadId.value)
  } catch (e) { toast.error(errorMessage(e, '加载会话失败')) }
}

function openCreate() {
  editing.value = null
  const first = modules.value[0]
  form.value = {
    name: first?.name || '',
    module: first?.id || 'database',
    description: first?.description || '',
    system_prompt: '',
    model_override: '',
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
    model_override: a.model_override || '',
    tool_ids: [...(a.tool_ids || [])]
  }
  modalOpen.value = true
}

function onModuleChange(mod: string) {
  form.value.module = mod
  const m = modules.value.find((x) => x.id === mod)
  if (m && !editing.value) {
    form.value.tool_ids = [...(m.default_tools || [])]
    if (!form.value.name) form.value.name = m.name
  }
}

function toggleTool(id: string) {
  const cur = form.value.tool_ids
  form.value.tool_ids = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]
}

async function saveAgent() {
  if (!form.value.name.trim()) {
    toast.warning('请填写名称')
    return
  }
  const mod = modules.value.find((m) => m.id === form.value.module)
  if (form.value.module === 'sandbox' && mod?.sandbox_available !== true) {
    toast.warning('云沙盒未配置，无法创建 Sandbox Agent')
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
    toast.error(errorMessage(e, '保存失败'))
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
    toast.error(errorMessage(e, '删除失败'))
  }
}

async function resetThread() {
  onStop()
  try {
    const th = await api.agentThreads.create(project.id, '云 Agent')
    threads.value.unshift(th)
    threadId.value = th.id
    chatMessages.value = []
    if (router?.replace) void router.replace({ query: { ...route.query, thread: th.id } })
  } catch (e) {
    toast.error(errorMessage(e, '新建会话失败'))
  }
}

async function onSend(text: string, mentions: { agent_id: string }[]) {
  const content = text.trim()
  if (!content || sending.value) return
  if (!threadId.value) await ensureThread()
  let used = mentions
  if (!used.length && activeAgent.value) used = [{ agent_id: activeAgent.value.id }]
  if (!used.length) { toast.warning('请先选择或 @ 一个 Agent'); return }
  draft.value = ''
  conv.start({ content, mentions: [...used] }, {
    onEnd: () => {
      void api.agentThreads.page(project.id).then((page) => { threads.value = page.threads; nextCursor.value = page.next_cursor }).catch(() => undefined)
    },
    onError: (e) => { toast.error(errorMessage(e, '运行失败')) }
  })
}

/** 重试：移除失败气泡后走后端 retry_of_run_id（BUG-03：不重复落 user 消息）。 */
function retryLast() {
  if (sending.value || !failedRunId.value) return
  const retryId = failedRunId.value
  // 移除末尾失败气泡（user 消息保留展示）。
  const last = chatMessages.value[chatMessages.value.length - 1]
  if (last?.role === 'assistant' && (last.error || last.canceled)) chatMessages.value.pop()
  conv.start({ content: '', mentions: [], retry_of_run_id: retryId }, {
    onEnd: () => {
      void api.agentThreads.page(project.id).then((page) => { threads.value = page.threads; nextCursor.value = page.next_cursor }).catch(() => undefined)
    },
    onError: (e) => { toast.error(errorMessage(e, '运行失败')) }
  })
}

function onStop() {
  if (sending.value) {
    const reply = chatMessages.value[chatMessages.value.length - 1]
    if (reply?.role === 'assistant') reply.canceled = true
  }
  conv.stop()
}

onMounted(() => {
  void bootstrap()
})
</script>
