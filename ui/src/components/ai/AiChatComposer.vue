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
    <Popover :open="mentionOpen && (mentionAgents.length > 0 || mentionSkills.length > 0)">
      <PopoverTrigger as-child>
        <span class="sr-only">mention</span>
      </PopoverTrigger>
      <PopoverContent class="w-72 p-0" side="top" align="start">
        <Command>
          <CommandList>
            <CommandEmpty>{{ t('components.aiComposer.noMentionMatch') }}</CommandEmpty>
            <CommandGroup v-if="filteredSkills.length">
              <p class="px-2 py-1 text-xs text-muted-foreground">{{ t('components.aiComposer.skillGroup') }}</p>
              <CommandItem
                v-for="skill in filteredSkills"
                :key="`skill:${skill.id}`"
                :value="`skill:${skill.name}`"
                :disabled="!skill.available"
                @select="() => pickSkill(skill)"
              >
                <component :is="iconForAgentModule(skill.id)" class="size-4 shrink-0 opacity-80" aria-hidden="true" />
                <span class="font-semibold text-primary">@</span>{{ skill.name }}
                <span v-if="!skill.available" class="ml-auto text-xs text-muted-foreground">{{ skill.unavailableReason || t('components.aiComposer.unavailable') }}</span>
              </CommandItem>
            </CommandGroup>
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
import { i18n } from '@/locales'
import { resolveSkillAlias, skillsInText } from '@/utils/agent-skills'
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
}

export interface MentionSkill {
  id: string
  name: string
  available: boolean
  unavailableReason?: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    disabled?: boolean
    sending?: boolean
    mentionAgents?: MentionAgent[]
    mentionSkills?: MentionSkill[]
  }>(),
  {
    placeholder: '',
    disabled: false,
    sending: false,
    mentionAgents: () => [],
    mentionSkills: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send', mentions: { agent_id: string }[], skills: string[]): void
  (e: 'stop'): void
}>()

const { t } = useI18n()

interface SelectedMention {
  start: number
  text: string
  agentId: string
}

interface SelectedSkill {
  start: number
  text: string
  skillId: string
}

const ta = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const selectedMentions = ref<SelectedMention[]>([])
const selectedSkills = ref<SelectedSkill[]>([])
const mentionError = ref('')
const filteredMentions = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  return (props.mentionAgents || []).filter((a) => !q || a.name.toLowerCase().includes(q) || (a.module || '').includes(q))
})
const filteredSkills = computed(() => {
  const q = mentionQuery.value.toLowerCase()
  return (props.mentionSkills || []).filter((skill) => !q || skill.name.toLowerCase().includes(q) || skill.id.includes(q))
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
  mentionOpen.value = props.mentionAgents.length > 0 || props.mentionSkills.length > 0
  mentionQuery.value = after
}

function pickSkill(skill: MentionSkill) {
  if (!skill.available) return
  const value = props.modelValue
  const at = value.lastIndexOf('@')
  if (at >= 0 && props.mentionSkills.some((item) => item.id === skill.id)) {
    selectedSkills.value = [...selectedSkills.value.filter((item) => item.start < at), { start: at, text: '@' + skill.name, skillId: skill.id }]
    selectedMentions.value = selectedMentions.value.filter((item) => item.start < at)
  }
  emit('update:modelValue', (at >= 0 ? value.slice(0, at) : value) + '@' + skill.name + ' ')
  mentionOpen.value = false
  nextTick(() => ta.value?.focus())
}

function pickMention(a: MentionAgent) {
  const value = props.modelValue
  const at = value.lastIndexOf('@')
  const next = (at >= 0 ? value.slice(0, at) : value) + '@' + a.name + ' '
  if (at >= 0 && props.mentionAgents.some((item) => item.id === a.id)) {
    selectedMentions.value = [...selectedMentions.value.filter((item) => item.start < at), { start: at, text: '@' + a.name, agentId: a.id }]
    selectedSkills.value = selectedSkills.value.filter((item) => item.start < at)
  }
  emit('update:modelValue', next)
  mentionOpen.value = false
  nextTick(() => ta.value?.focus())
}

function namesForAgent(agent: MentionAgent): string[] {
  const names = [agent.name]
  if (agent.legacyName && !names.includes(agent.legacyName)) names.push(agent.legacyName)
  if (agent.builtinKey) {
    if (agent.builtinKey === 'kv') names.push('KV助手')
    for (const locale of ['zh-CN', 'en-US'] as const) {
      const translated = i18n.global.t(`pages.agents.builtinNames.${agent.builtinKey}`, {}, { locale })
      if (!names.includes(translated)) names.push(translated)
    }
  }
  return names
}

function agentNames() {
  return props.mentionAgents.flatMap(namesForAgent)
}

function requiresMentionSelection(text: string): boolean {
  for (const match of text.matchAll(/@([^\s@]+)/g)) {
    if (resolveSkillAlias(match[1], agentNames())) continue
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
  emit('send', mentionsForSend(props.modelValue), skillsForSend(props.modelValue))
}

function mentionsForSend(text: string) {
  const names = agentNames()
  const found: { agent_id: string }[] = []
  for (const item of selectedMentions.value) {
    if (text.slice(item.start, item.start + item.text.length) !== item.text) continue
    const after = text[item.start + item.text.length]
    if (after && !/\s/.test(after)) continue
    const agent = props.mentionAgents.find((candidate) => candidate.id === item.agentId)
    if (agent && !found.some((entry) => entry.agent_id === agent.id)) found.push({ agent_id: agent.id })
  }
  for (const match of text.matchAll(/@([^\s@]+)/g)) {
    if (selectedSkills.value.some((item) => item.start === match.index && item.text === match[0])) continue
    if (selectedMentions.value.some((item) => item.start === match.index && text.slice(item.start, item.start + item.text.length) === item.text && (!text[item.start + item.text.length] || /\s/.test(text[item.start + item.text.length])))) continue
    const matches = props.mentionAgents.filter((a) => namesForAgent(a).includes(match[1]))
    const agent = matches.length === 1 ? matches[0] : undefined
    if (agent && !resolveSkillAlias(match[1], names) && !found.some((entry) => entry.agent_id === agent.id)) found.push({ agent_id: agent.id })
  }
  return found
}

function skillsForSend(text: string) {
  const selectedAgents = selectedMentions.value.filter((item) => text.slice(item.start, item.start + item.text.length) === item.text && (!text[item.start + item.text.length] || /\s/.test(text[item.start + item.text.length])))
  const skillText = selectedAgents.reduce((result, item) => result.slice(0, item.start) + ' '.repeat(item.text.length) + result.slice(item.start + item.text.length), text)
  const ids = skillsInText(skillText, agentNames())
  for (const item of selectedSkills.value) {
    if (text.slice(item.start, item.start + item.text.length) === item.text && !ids.includes(item.skillId)) {
      const after = text[item.start + item.text.length]
      if (!after || /\s/.test(after)) ids.push(item.skillId)
    }
  }
  return ids
}

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  mentionError.value = ''
  selectedMentions.value = selectedMentions.value.filter((item) => el.value.slice(item.start, item.start + item.text.length) === item.text && (!el.value[item.start + item.text.length] || /\s/.test(el.value[item.start + item.text.length])))
  selectedSkills.value = selectedSkills.value.filter((item) => el.value.slice(item.start, item.start + item.text.length) === item.text && (!el.value[item.start + item.text.length] || /\s/.test(el.value[item.start + item.text.length])))
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
    selectedSkills.value = selectedSkills.value.filter((item) => value.slice(item.start, item.start + item.text.length) === item.text && (!value[item.start + item.text.length] || /\s/.test(value[item.start + item.text.length])))
    autosize()
  }
)

/** 聚焦输入框并把光标放到末尾；供宿主在点提示词后调用（planv6.0 §3.5）。 */
function focus() {
  nextTick(() => {
    const el = ta.value
    if (!el) return
    el.focus()
    const end = el.value.length
    el.setSelectionRange(end, end)
  })
}

defineExpose({ focus })
</script>
