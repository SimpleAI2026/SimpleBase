<template>
  <!-- AgentTagModal：标签新建/编辑弹窗。
       名称唯一性、助手归属、有序提示词；409 版本冲突提示覆盖重载。 -->
  <SbModal
    :open="open"
    :title="editing ? t('components.agentTagModal.editTitle', { name: initial.name }) : t('components.agentTagModal.createTitle')"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !name.trim() }"
    @ok="submit"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <div v-if="conflicted" class="mb-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive" role="alert" data-tag-conflict>
      {{ t('components.agentTagModal.conflict') }}
      <Button size="xs" variant="outline" class="ml-2" @click="emit('reload')">{{ t('components.agentTagModal.reload') }}</Button>
    </div>
    <FieldGroup>
      <Field>
        <FieldLabel for="agent-tag-name">{{ t('components.agentTagModal.name') }}</FieldLabel>
        <Input id="agent-tag-name" v-model="name" placeholder="alerts" data-tag-name-input />
      </Field>
      <Field>
        <FieldLabel>{{ t('components.agentTagModal.agents') }}</FieldLabel>
        <div class="flex flex-col gap-1 rounded-md border border-border p-2" data-tag-agent-list>
          <label
            v-for="a in agents"
            :key="a.id"
            class="flex min-h-8 cursor-pointer items-center gap-2 text-xs"
          >
            <input
              type="checkbox"
              class="size-3.5 accent-[var(--primary)]"
              :checked="agentIds.includes(a.id)"
              :aria-label="a.name"
              :data-tag-agent-check="a.id"
              @change="toggleAgent(a.id)"
            />
            <component :is="iconForAgentModule(a.module)" class="size-3.5 shrink-0 opacity-80" aria-hidden="true" />
            <span class="min-w-0 flex-1 truncate">{{ a.name }}</span>
            <span class="shrink-0 text-muted-foreground">{{ a.module }}</span>
          </label>
          <p v-if="!agents.length" class="px-1 py-2 text-xs text-muted-foreground">{{ t('pages.agents.emptyListDesc') }}</p>
        </div>
      </Field>
      <!-- 路由提示词：只决定分工，不授予权限。 -->
      <Field>
        <FieldLabel for="agent-tag-routing">{{ t('components.agentTagModal.routingPrompt') }}</FieldLabel>
        <textarea
          id="agent-tag-routing"
          v-model="routingPrompt"
          rows="4"
          class="w-full resize-y rounded-md border border-border bg-transparent px-2 py-1.5 text-xs leading-relaxed"
          :placeholder="t('components.agentTagModal.routingPlaceholder')"
          data-tag-routing-prompt
        />
        <p class="mt-1 text-[11px] leading-relaxed text-muted-foreground">{{ t('components.agentTagModal.routingHint') }}</p>
      </Field>

      <!-- 兜底助手与人工策略。 -->
      <div class="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel for="agent-tag-fallback">{{ t('components.agentTagModal.fallback') }}</FieldLabel>
          <Select :model-value="fallbackAgentId || NONE" @update:model-value="setFallback">
            <SelectTrigger id="agent-tag-fallback" class="w-full" size="sm" data-tag-fallback>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem :value="NONE">{{ t('components.agentTagModal.fallbackNone') }}</SelectItem>
              <SelectItem v-for="a in memberAgents" :key="a.id" :value="a.id">{{ a.name }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel for="agent-tag-human">{{ t('components.agentTagModal.humanPolicy') }}</FieldLabel>
          <Select :model-value="humanPolicy || NONE" @update:model-value="setHumanPolicy">
            <SelectTrigger id="agent-tag-human" class="w-full" size="sm" data-tag-human-policy>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem :value="NONE">{{ t('components.agentTagModal.humanAllowed') }}</SelectItem>
              <SelectItem value="disabled">{{ t('components.agentTagModal.humanDisabled') }}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>

      <Field>
        <FieldLabel>{{ t('components.agentTagModal.prompts') }}</FieldLabel>
        <div class="flex flex-col gap-2" data-tag-prompt-list>
          <div v-for="(p, i) in prompts" :key="p.id" class="flex flex-col gap-1 rounded-md border border-border p-2">
            <div class="flex items-center gap-1">
              <Input v-model="p.title" class="h-8 text-xs" :placeholder="t('components.agentTagModal.promptTitle')" :data-tag-prompt-title="i" />
              <Button size="icon-sm" variant="ghost" class="shrink-0" :title="t('components.agentTagModal.moveUp')" :disabled="i === 0" @click="movePrompt(i, -1)">
                <ChevronUpIcon />
              </Button>
              <Button size="icon-sm" variant="ghost" class="shrink-0" :title="t('components.agentTagModal.moveDown')" :disabled="i === prompts.length - 1" @click="movePrompt(i, 1)">
                <ChevronDownIcon />
              </Button>
              <Button size="icon-sm" variant="ghost" class="shrink-0 text-destructive" :title="t('components.agentTagModal.removePrompt')" @click="prompts.splice(i, 1)">
                <Trash2Icon />
              </Button>
            </div>
            <textarea
              v-model="p.content"
              rows="2"
              class="w-full resize-none rounded-md border border-border bg-transparent px-2 py-1 text-xs"
              :placeholder="t('components.agentTagModal.promptContent')"
              :data-tag-prompt-content="i"
            />
          </div>
          <Button size="xs" variant="outline" data-tag-add-prompt @click="addPrompt">
            <PlusIcon class="size-3" /> {{ t('components.agentTagModal.addPrompt') }}
          </Button>
        </div>
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 标签编辑弹窗：名称 + 助手多选 + 提示词有序列表。
 * 提交走宿主；409 版本冲突由宿主置 conflicted，用户点「重新加载」丢弃本地修改。
 */
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDownIcon, ChevronUpIcon, PlusIcon, Trash2Icon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SbModal from '@/components/modal/SbModal.vue'
import { iconForAgentModule } from '@/components/nav-icons'
import type { AgentTag, AgentTagInput, MentionAgentLike } from '@/services/types'

const props = defineProps<{
  open: boolean
  /** 编辑中的标签（null = 新建）。 */
  tag: AgentTag | null
  agents: MentionAgentLike[]
  saving?: boolean
  /** 版本冲突（409）：提示重新加载。 */
  conflicted?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'save', input: AgentTagInput): void
  (e: 'reload'): void
}>()

const { t } = useI18n()

/** Select 不允许空串作为 value，用哨兵值表示「不设置」。 */
const NONE = '__none__'

const name = ref('')
const agentIds = ref<string[]>([])
/** 群路由提示词与兜底/人工策略。 */
const routingPrompt = ref('')
const fallbackAgentId = ref('')
const humanPolicy = ref<'' | 'disabled'>('')
/** 本地提示词草稿：直接改（数组元素是 reactive 容器内对象）。 */
interface PromptDraft { id: string; title: string; content: string }
const prompts = reactive<PromptDraft[]>([])
const initial = reactive({ name: '' })
const editing = ref(false)

/** 兜底下拉只列当前已选成员：兜底越出成员范围等于绕过成员控制。 */
const memberAgents = computed(() => props.agents.filter((a) => agentIds.value.includes(a.id)))

watch(
  () => [props.open, props.tag] as const,
  ([open, tag]) => {
    if (!open) return
    editing.value = !!tag
    name.value = tag?.name || ''
    initial.name = tag?.name || ''
    agentIds.value = [...(tag?.agentIds || [])]
    routingPrompt.value = tag?.routingPrompt || ''
    fallbackAgentId.value = tag?.fallbackAgentId || ''
    humanPolicy.value = tag?.humanPolicy === 'disabled' ? 'disabled' : ''
    prompts.splice(0, prompts.length, ...(tag?.prompts || []).map((p) => ({ ...p })))
  },
  { immediate: true }
)

// 成员被取消勾选时，兜底指向非成员要一起清掉，避免提交被后端拒绝。
watch(agentIds, (ids) => {
  if (fallbackAgentId.value && !ids.includes(fallbackAgentId.value)) fallbackAgentId.value = ''
})

function setFallback(value: unknown) {
  const v = String(value)
  fallbackAgentId.value = v === NONE ? '' : v
}

function setHumanPolicy(value: unknown) {
  humanPolicy.value = value === 'disabled' ? 'disabled' : ''
}

function toggleAgent(id: string) {
  agentIds.value = agentIds.value.includes(id) ? agentIds.value.filter((x) => x !== id) : [...agentIds.value, id]
}

function addPrompt() {
  prompts.push({ id: crypto.randomUUID(), title: '', content: '' })
}

function movePrompt(index: number, delta: number) {
  const next = index + delta
  if (next < 0 || next >= prompts.length) return
  const [item] = prompts.splice(index, 1)
  prompts.splice(next, 0, item)
}

function submit() {
  const trimmed = name.value.trim()
  if (!trimmed) return
  const filled = prompts
    .map((p) => ({ id: p.id, title: p.title.trim(), content: p.content.trim() }))
    .filter((p) => p.title || p.content)
  emit('save', {
    name: trimmed,
    agentIds: [...agentIds.value],
    prompts: filled,
    routingPrompt: routingPrompt.value.trim(),
    fallbackAgentId: fallbackAgentId.value,
    humanPolicy: humanPolicy.value === 'disabled' ? 'disabled' : undefined,
    expectedVersion: props.tag?.version
  })
}
</script>
