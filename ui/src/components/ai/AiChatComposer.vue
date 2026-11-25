<template>
  <div
    class="relative flex items-center gap-2 px-3 py-2.5"
    :class="disabled ? 'opacity-60' : ''"
    :style="{
      background: 'var(--composer-bg)',
      borderRadius: 'var(--composer-radius)',
      border: '1px solid var(--border)',
    }"
  >
    <textarea
      ref="ta"
      class="max-h-55 min-h-6 min-w-0 flex-1 resize-none border-0 bg-transparent py-0 font-sans text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground"
      :value="modelValue"
      :placeholder="placeholder || t('components.aiComposer.placeholder')"
      :disabled="disabled || sending"
      rows="1"
      @input="onInput"
      @keydown="onKeydown"
    />
    <span v-if="mentionError" role="alert" class="absolute bottom-full left-0 mb-2 rounded-md border border-border bg-popover px-3 py-2 text-xs text-foreground shadow-sm">{{ mentionError }}</span>
    <Popover :open="mentionOpen && mentionAgents.length > 0">
      <PopoverTrigger as-child>
        <span class="sr-only">mention</span>
      </PopoverTrigger>
      <PopoverContent class="w-72 p-0" side="top" align="start">
        <Command>
          <CommandList>
            <CommandEmpty>{{ t('components.aiComposer.noMentionMatch') }}</CommandEmpty>
            <CommandGroup v-if="filteredMentions.length">
              <p class="px-2 py-1 text-xs text-muted-foreground">{{ t('components.aiComposer.agentGroup') }}</p>
              <CommandItem
                v-for="a in filteredMentions"
                :key="a.id"
                :value="`agent:${a.name}:${a.id}`"
                @select="() => pickMention(a)"
              >
                <component :is="iconForAgentModule(a.module)" class="size-4 shrink-0 opacity-80" aria-hidden="true" />
                <span class="font-semibold text-primary">@</span>{{ a.name }}
                <span class="ml-auto text-xs text-muted-foreground">{{ a.module }}</span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>

    <slot name="actions" />

    <Button
      v-if="sending"
      size="icon"
      class="rounded-full bg-destructive text-white hover:bg-destructive/95 dark:text-background dark:hover:bg-destructive/90"
      :title="t('components.aiComposer.stop')"
      :aria-label="t('components.aiComposer.stopAria')"
      @click="$emit('stop')"
    >
      <SquareIcon />
    </Button>
    <Button
      v-else
      size="icon"
      class="rounded-full bg-primary text-primary-foreground hover:bg-primary/90"
      :disabled="disabled || !modelValue.trim()"
      :title="t('components.aiComposer.send')"
      :aria-label="t('components.aiComposer.sendAria')"
      @click="sendMessage"
    >
      <ArrowUpIcon />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUpIcon, SquareIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { iconForAgentModule } from '@/components/nav-icons'
import { builtinAliases } from '@/utils/agent-names'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export interface MentionAgent {
  id: string
  name: string
  module?: string
  /** 服务端保存的旧内置名称，语言切换后仍可识别老草稿。 */
  legacyName?: string
  builtinKey?: string
  /**
   * 点名目标类型：'human' 表示「当前人」，发送时转成
   * {kind:'human', id:'me'}，由服务端解析为登录用户。缺省是助手。
   */
  kind?: 'agent' | 'human'
}

/** 发送时的点名声明的目标（带 kind；旧消费方只读 agent_id 仍然可用）。 */
export interface MentionSendTarget {
  agent_id: string
  kind?: 'agent' | 'human'
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    sending?: boolean
    mentionAgents?: MentionAgent[]
  }>(),
  {
    placeholder: '',
    disabled: false,
    sending: false,
    mentionAgents: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send', mentions: MentionSendTarget[]): void
  (e: 'stop'): void
}>()

const { t } = useI18n()

interface SelectedMention {
  start: number
  text: string
  agentId: string
}

const ta = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const selectedMentions = ref<SelectedMention[]>([])
const mentionError = ref('')
const filteredMentions = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  return (props.mentionAgents || []).filter((a) => !q || a.name.toLowerCase().includes(q) || (a.module || '').includes(q))
})

function parseMentionQuery(value: string) {
  const at = value.lastIndexOf('@')
  if (at < 0) {
    mentionOpen.value = false
    mentionQuery.value = ''
    return
  }
  const after = value.slice(at + 1)
  if (after.includes(' ') || after.includes('\n')) {
    mentionOpen.value = false
    return
  }
  mentionOpen.value = props.mentionAgents.length > 0
  mentionQuery.value = after
}

function pickMention(a: MentionAgent) {
  const value = props.modelValue
  const at = value.lastIndexOf('@')
  const next = (at >= 0 ? value.slice(0, at) : value) + '@' + a.name + ' '
  if (at >= 0 && props.mentionAgents.some((item) => item.id === a.id)) {
    selectedMentions.value = [...selectedMentions.value.filter((item) => item.start < at), { start: at, text: '@' + a.name, agentId: a.id }]
  }
  emit('update:modelValue', next)
  mentionOpen.value = false
  nextTick(() => ta.value?.focus())
}

/**
 * 供宿主从助手条注入点名（点击助手 = 在草稿开头插入 @名称 ）。
 * 无 @ 前缀时在开头插入；已有文本自动补一个空格分隔。
 */
function insertMention(a: MentionAgent) {
  const value = props.modelValue
  const prefix = value && !value.startsWith(' ') ? value + ' ' : value
  const at = prefix.length
  const next = prefix + '@' + a.name + ' '
  selectedMentions.value = [...selectedMentions.value, { start: at, text: '@' + a.name, agentId: a.id }]
  emit('update:modelValue', next)
  nextTick(() => ta.value?.focus())
}

/** 清空已选 mention 并从文本移除对应 @名称 片段。 */
function clearMentions() {
  const value = props.modelValue
  if (!selectedMentions.value.length) {
    emit('update:modelValue', value)
    return
  }
  let next = value
  for (const item of [...selectedMentions.value].sort((a, b) => b.start - a.start)) {
    if (next.slice(item.start, item.start + item.text.length) === item.text) {
      next = next.slice(0, item.start) + next.slice(item.start + item.text.length)
    }
  }
  selectedMentions.value = []
  emit('update:modelValue', next)
  mentionOpen.value = false
}

function namesForAgent(agent: MentionAgent): string[] {
  const names = [agent.name]
  if (agent.legacyName && !names.includes(agent.legacyName)) names.push(agent.legacyName)
  if (agent.builtinKey) {
    for (const alias of builtinAliases(agent.builtinKey)) {
      if (!names.includes(alias)) names.push(alias)
    }
  }
  return names
}

function agentNames() {
  return props.mentionAgents.flatMap(namesForAgent)
}

function requiresMentionSelection(text: string): boolean {
  for (const match of text.matchAll(/@([^\s@]+)/g)) {
    const agents = props.mentionAgents.filter((item) => namesForAgent(item).includes(match[1]))
    if (agents.length > 1 && !selectedMentions.value.some((item) => item.start === match.index && item.text === match[0])) return true
  }
  for (const agent of props.mentionAgents) {
    if (!agent.name.includes(' ')) continue
    const token = '@' + agent.name
    const start = text.indexOf(token)
    if (start < 0) continue
    const selected = selectedMentions.value.some((item) => item.start === start && item.text === token && item.agentId === agent.id)
    if (!selected) return true
  }
  return false
}

function sendMessage() {
  if (requiresMentionSelection(props.modelValue)) {
    mentionError.value = t('components.aiComposer.selectMention')
    return
  }
  mentionError.value = ''
  emit('send', mentionsForSend(props.modelValue))
}

function mentionsForSend(text: string): MentionSendTarget[] {
  const found: MentionSendTarget[] = []
  const push = (agent: MentionAgent) => {
    if (found.some((entry) => entry.agent_id === agent.id)) return
    // v2：人目标带 kind='human'，页面据此转成 {kind:'human', id:'me'}。
    if (agent.kind === 'human') found.push({ agent_id: agent.id, kind: 'human' })
    else found.push({ agent_id: agent.id })
  }
  for (const item of selectedMentions.value) {
    if (text.slice(item.start, item.start + item.text.length) !== item.text) continue
    const after = text[item.start + item.text.length]
    if (after && !/\s/.test(after)) continue
    const agent = props.mentionAgents.find((candidate) => candidate.id === item.agentId)
    if (agent) push(agent)
  }
  for (const match of text.matchAll(/@([^\s@]+)/g)) {
    if (selectedMentions.value.some((item) => item.start === match.index && text.slice(item.start, item.start + item.text.length) === item.text && (!text[item.start + item.text.length] || /\s/.test(text[item.start + item.text.length])))) continue
    const matches = props.mentionAgents.filter((a) => namesForAgent(a).includes(match[1]))
    const agent = matches.length === 1 ? matches[0] : undefined
    if (agent) push(agent)
  }
  return found
}

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  mentionError.value = ''
  selectedMentions.value = selectedMentions.value.filter((item) => el.value.slice(item.start, item.start + item.text.length) === item.text && (!el.value[item.start + item.text.length] || /\s/.test(el.value[item.start + item.text.length])))
  emit('update:modelValue', el.value)
  parseMentionQuery(el.value)
  autosize()
}

function onKeydown(e: KeyboardEvent) {
  if (mentionOpen.value && e.key === 'Escape') {
    mentionOpen.value = false
    return
  }
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (!props.sending && props.modelValue.trim()) sendMessage()
  }
}

function autosize() {
  nextTick(() => {
    const el = ta.value
    if (!el) return
    el.style.height = 'auto'
    const max = 220
    el.style.height = `${Math.min(el.scrollHeight, max)}px`
  })
}

watch(
  () => props.modelValue,
  (value) => {
    selectedMentions.value = selectedMentions.value.filter((item) => value.slice(item.start, item.start + item.text.length) === item.text && (!value[item.start + item.text.length] || /\s/.test(value[item.start + item.text.length])))
    autosize()
  }
)

/** 聚焦输入框并把光标放到末尾；供宿主在点提示词后调用。 */
function focus() {
  nextTick(() => {
    const el = ta.value
    if (!el) return
    el.focus()
    const end = el.value.length
    el.setSelectionRange(end, end)
  })
}

defineExpose({ focus, insertMention, clearMentions })
</script>
