<template>
  <ProjectScope>
    <PageContainer subtitle="按模块的只读 Agent；composer 输入 @ 点名">
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
            <SbEmptyState v-if="!loading && !agents.length" description="还没有 Agent" action-text="创建" @action="openCreate" />
            <div class="flex flex-col gap-3">
              <button
                v-for="a in agents"
                :key="a.id"
                type="button"
                class="w-full rounded-xl border p-4 text-left transition-all cursor-pointer"
                :class="a.id === activeId ? 'border-primary/60 bg-primary/8 shadow-xs' : 'border-border bg-card hover:bg-muted/40 hover:border-border'"
                @click="activeId = a.id"
              >
                <div class="flex items-center justify-between gap-2">
                  <strong class="text-sm font-semibold text-foreground">{{ a.name }}</strong>
                  <Badge variant="secondary" class="text-[11px]">{{ a.module }}</Badge>
                </div>
                <div class="mt-1.5 text-xs leading-relaxed text-muted-foreground line-clamp-2">{{ a.description || '无描述' }}</div>
                <div v-if="scheduleByAgent[a.id]" class="mt-1.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <ClockIcon class="size-3" />
                  <span>{{ scheduleSummary(scheduleByAgent[a.id]) }}</span>
                  <span v-if="scheduleByAgent[a.id]?.enabled" class="size-1.5 rounded-full bg-emerald-500" />
                  <span v-else class="size-1.5 rounded-full bg-muted-foreground/40" />
                </div>
                <div class="mt-3 flex gap-1 justify-end border-t border-border/60 pt-3" @click.stop>
                  <Button variant="ghost" size="xs" @click="openEdit(a)">编辑</Button>
                  <Button variant="ghost" size="xs" @click="openSchedule(a)">定时</Button>
                  <ConfirmAction title="确认删除该 Agent？" @confirm="removeAgent(a)">
                    <Button variant="ghost" size="xs" class="text-destructive hover:bg-destructive/10">删除</Button>
                  </ConfirmAction>
                </div>
              </button>
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
          <CardContent class="p-4 pt-4">
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
                <Button variant="outline" size="sm" class="shrink-0" :disabled="!chatMessages.length && !sending" @click="resetThread">新会话</Button>
                <span class="min-w-0 truncate text-xs text-muted-foreground">点名 {{ activeAgent ? '@' + activeAgent.name : '一个 Agent' }} 后发送；工具只读</span>
              </template>
              <template #empty>
                <SbEmptyState v-if="!chatMessages.length" description="用 @ 点名左侧 Agent，询问数据库、对象或日志" />
              </template>
            </AiChat>
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
                  <SelectItem v-for="opt in moduleOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
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
            <FieldLabel>只读工具</FieldLabel>
            <div class="flex flex-wrap gap-2">
              <Badge
                v-for="id in toolOptions"
                :key="id.value"
                :variant="form.tool_ids.includes(id.value) ? 'default' : 'outline'"
                class="cursor-pointer"
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
import { toast } from 'vue-sonner'
import { ClockIcon, PlusIcon, RefreshCwIcon } from '@lucide/vue'
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
import type { AgentModuleInfo, CloudAgent, LlmStreamConnection } from '../services/api'
import { useProjectStore } from '../stores/project'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import SbModal from '../components/modal/SbModal.vue'
import AiChat from '../components/ai/AiChat.vue'
import AgentScheduleModal from '../components/ai/AgentScheduleModal.vue'
import type { ChatMsg } from '../composables/useAiChat'
import type { AgentSchedule } from '../services/types'

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
  await loadSchedules()
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
  onStop()
  scheduleModalOpen.value = false
  threadId.value = threadIdToView
  try {
    const msgs = await api.agentThreads.messages(project.id, threadId.value)
    chatMessages.value = msgs.map((m) => ({
      role: m.role === 'assistant' ? 'assistant' : 'user',
      content: m.content,
      toolCalls: m.tool_calls
    }))
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载会话失败')
  }
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
    const list = await api.agentThreads.list(project.id)
    if (list.length) {
      threadId.value = list[0].id
    } else {
      const th = await api.agentThreads.create(project.id, '云 Agent')
      threadId.value = th.id
    }
    const msgs = await api.agentThreads.messages(project.id, threadId.value)
    chatMessages.value = msgs.map((m) => ({
      role: m.role === 'assistant' ? 'assistant' : 'user',
      content: m.content,
      toolCalls: m.tool_calls
    }))
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载会话失败')
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
    threadId.value = th.id
    chatMessages.value = []
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '新建会话失败')
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
    toast.warning('请先选择或 @ 一个 Agent')
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
        if (e) toast.error(e instanceof Error ? e.message : '运行失败')
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
