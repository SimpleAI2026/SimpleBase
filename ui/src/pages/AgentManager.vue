<template>
  <ProjectScope>
    <PageContainer subtitle="按模块的 Agent；composer 输入 @ 点名；工具默认只读，Sandbox 在云端隔离环境执行">
      <div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-[300px_minmax(0,1fr)]">
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
          <CardContent class="p-4">
            <SbEmptyState v-if="!loading && !agents.length" :icon="BotIcon" description="还没有 Agent" action-text="创建" @action="openCreate" />
            <div class="flex flex-col gap-3">
              <article
                v-for="a in agents"
                :key="a.id"
                class="rounded-xl border bg-card p-4 transition-colors"
                :class="a.id === activeId ? 'border-primary/60 bg-primary/8 shadow-xs' : 'border-border hover:border-primary/30'"
              >
                <button
                  type="button"
                  class="block w-full rounded-md text-left"
                  :aria-label="`选择 Agent ${a.name}`"
                  :aria-pressed="a.id === activeId"
                  @click="activeId = a.id"
                >
                  <span class="flex items-center justify-between gap-2">
                    <strong class="text-sm font-semibold text-foreground">{{ a.name }}</strong>
                    <Badge variant="secondary" class="text-xs">{{ a.module }}</Badge>
                  </span>
                  <span class="mt-1.5 block text-xs leading-relaxed text-muted-foreground line-clamp-2">{{ a.description || '无描述' }}</span>
                  <span v-if="scheduleByAgent[a.id]" class="mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <ClockIcon aria-hidden="true" class="size-3" />
                    <span>{{ scheduleSummary(scheduleByAgent[a.id]) }}</span>
                    <span v-if="scheduleByAgent[a.id]?.enabled" aria-hidden="true" class="size-1.5 rounded-full bg-success" />
                    <span v-else aria-hidden="true" class="size-1.5 rounded-full bg-muted-foreground/40" />
                  </span>
                </button>
                <div class="mt-3 flex flex-wrap gap-1 justify-end border-t border-border/60 pt-3">
                  <Button variant="ghost" size="xs" @click="openEdit(a)">编辑</Button>
                  <Button variant="ghost" size="xs" @click="openSchedule(a)">定时</Button>
                  <ConfirmAction title="确认删除该 Agent？" @confirm="removeAgent(a)">
                    <Button variant="destructiveGhost" size="xs">删除</Button>
                  </ConfirmAction>
                </div>
              </article>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader class="border-b">
            <CardTitle class="flex items-center gap-2">
              {{ activeAgent ? '@' + activeAgent.name : '对话' }}
              <Badge v-if="activeAgent" variant="secondary" class="text-xs">{{ activeAgent.module }}</Badge>
            </CardTitle>
          </CardHeader>
          <CardContent class="grid min-w-0 gap-4 p-4 pt-4 md:grid-cols-[220px_minmax(0,1fr)]">
            <AgentThreadList :threads="threads" :active-id="threadId" :next-cursor="nextCursor" :busy="sending"
              @create="resetThread" @select="selectThread" @rename="renameThread" @remove="removeThread" @more="loadMoreThreads" />
            <div class="min-w-0">
              <p v-if="runState.statusText.value" role="status" class="mb-2 text-xs text-muted-foreground">{{ runState.statusText.value }}</p>
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
              @retry="retryLast"
            >
              <template #toolbar>
                <span class="min-w-0 truncate text-xs text-muted-foreground">{{ toolsHint }}</span>
                <router-link v-if="activeAgentHasSandboxTools" :to="{ name: 'sandboxes' }" class="shrink-0 text-xs text-primary hover:underline">在云沙盒页查看</router-link>
              </template>
              <template #empty>
                <SbEmptyState v-if="!chatMessages.length" :icon="BotIcon" description="用 @ 点名左侧 Agent，询问数据库、对象或日志；Sandbox Agent 可在云端环境运行代码" />
              </template>
            </AiChat>
            </div>
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
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { BotIcon, ClockIcon, PlusIcon, RefreshCwIcon } from '@lucide/vue'
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
import ConfirmAction from '../components/ConfirmAction.vue'
import SbModal from '../components/modal/SbModal.vue'
import AiChat from '../components/ai/AiChat.vue'
import AgentScheduleModal from '../components/ai/AgentScheduleModal.vue'
import AgentThreadList from '../components/ai/AgentThreadList.vue'
import { useAgentRun } from '../composables/useAgentRun'
import type { ChatMsg } from '../composables/useAiChat'
import type { AgentSchedule } from '../services/types'

const project = useProjectStore()
const route = useRoute()
const router = useRouter()
const runState = useAgentRun(() => project.id)
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
const currentRunId = ref('')
const chatMessages = ref<ChatMsg[]>([])
const sending = ref(false)
const lastRequest = ref<{ content: string; mentions: { agent_id: string }[] } | null>(null)

const scheduleByAgent = ref<Record<string, AgentSchedule>>({})
const scheduleModalOpen = ref(false)
const scheduleAgent = ref<CloudAgent | null>(null)

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
    const settings = await api.llmSettings.get(project.id)
    defaultModel.value = settings.defaultModel || ''
    modelOptions.value = defaultModel.value ? [defaultModel.value] : []
  } catch { defaultModel.value = '' }
}

async function loadSchedules() {
  try {
    const list = await api.agentSchedules.list(project.id)
    const map: Record<string, AgentSchedule> = {}
    for (const s of list) map[s.agent_id] = s
    scheduleByAgent.value = map
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载定时任务失败')
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
    toolCalls: m.tool_calls
  }))
}

async function selectThread(id: string) {
  if (!id) return
  onStop()
  threadId.value = id
  chatMessages.value = []
  if (router?.replace) void router.replace({ query: { ...route.query, thread: id } })
  try { await loadThreadMessages(id) }
  catch (e) { toast.error(e instanceof Error ? e.message : '加载会话失败') }
}

async function loadMoreThreads() {
  if (!nextCursor.value) return
  try {
    const page = await api.agentThreads.page(project.id, 50, nextCursor.value)
    threads.value.push(...page.threads)
    nextCursor.value = page.next_cursor
  } catch (e) { toast.error(e instanceof Error ? e.message : '加载会话失败') }
}

async function renameThread(id: string, title: string) {
  try {
    const renamed = await api.agentThreads.rename(project.id, id, title)
    threads.value = threads.value.map((th) => th.id === id ? renamed : th)
  } catch (e) { toast.error(e instanceof Error ? e.message : '重命名失败') }
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
  } catch (e) { toast.error(e instanceof Error ? e.message : '删除会话失败') }
}

async function loadModules() {
  try {
    modules.value = await api.agents.modules(project.id)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载模块失败')
  }
}

async function loadAgents() {
  loading.value = true
  try {
    agents.value = await api.agents.list(project.id)
    if (!activeId.value && agents.value.length) activeId.value = agents.value[0].id
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载 Agent 失败')
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
  } catch (e) { toast.error(e instanceof Error ? e.message : '加载会话失败') }
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
    toast.error(e instanceof Error ? e.message : '保存失败')
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
    toast.error(e instanceof Error ? e.message : '删除失败')
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
    toast.error(e instanceof Error ? e.message : '新建会话失败')
  }
}

async function onSend(text: string, mentions: { agent_id: string }[]) {
  const content = text.trim()
  if (!content || sending.value) return
  if (!threadId.value) await ensureThread()
  let used = mentions
  if (!used.length && activeAgent.value) used = [{ agent_id: activeAgent.value.id }]
  if (!used.length) { toast.warning('请先选择或 @ 一个 Agent'); return }
  lastRequest.value = { content, mentions: [...used] }
  chatMessages.value.push({ role: 'user', content })
  const reply: ChatMsg = { role: 'assistant', content: '', toolCalls: [] }
  chatMessages.value.push(reply)
  sending.value = true
  currentRunId.value = ''
  runState.start((handlers) => api.agentThreads.streamRun(
    project.id, threadId.value, { content, mentions: used, stream: true }, handlers
  ), {
    onRun: (id) => { currentRunId.value = id },
    onThinking: (_ms, content) => { if (content) reply.thinking = (reply.thinking || '') + content },
    onToken: (t) => { reply.content += t },
    onToolCall: (name, args, callId) => {
      if (callId && reply.toolCalls?.some((card) => card.call_id === callId)) return
      reply.toolCalls = [...(reply.toolCalls || []), { call_id: callId, name, arguments: args }]
    },
    onToolResult: (name, body, callId, durationMs) => {
      const cards = reply.toolCalls || []
      const target = callId ? cards.find((card) => card.call_id === callId) : [...cards].reverse().find((card) => card.name === name && !card.content)
      if (target) { target.content = body; target.duration_ms = durationMs }
      else cards.push({ call_id: callId, name, content: body, duration_ms: durationMs })
      reply.toolCalls = [...cards]
    },
    onEnd: (reason) => {
      sending.value = false
      if (reason === 'canceled') reply.canceled = true
      void api.agentThreads.page(project.id).then((page) => { threads.value = page.threads; nextCursor.value = page.next_cursor }).catch(() => undefined)
    },
    onError: (error) => {
      sending.value = false
      const e = error as Error & { code?: string }
      const labels: Record<string, string> = {
        llm_auth_failed: '模型服务鉴权失败', llm_rate_limited: '模型服务限流，请稍后重试',
        llm_timeout: '模型响应超时', llm_model_not_allowed: '模型不在允许列表中',
        agent_thread_busy: '当前会话仍在运行', quota_exceeded: '模型调用配额已用完'
      }
      reply.error = labels[e?.code || ''] || (e instanceof Error ? e.message : '运行失败')
      toast.error(reply.error)
    }
  })
}

function retryLast() {
  if (!lastRequest.value || sending.value) return
  const last = lastRequest.value
  const reply = chatMessages.value[chatMessages.value.length - 1]
  if (reply?.role === 'assistant' && reply.error) chatMessages.value.pop()
  if (chatMessages.value[chatMessages.value.length - 1]?.role === 'user') chatMessages.value.pop()
  void onSend(last.content, last.mentions)
}

function onStop() {
  if (sending.value) {
    const reply = chatMessages.value[chatMessages.value.length - 1]
    if (reply?.role === 'assistant') reply.canceled = true
  }
  runState.stop()
  sending.value = false
}

onMounted(() => {
  void bootstrap()
})
</script>
