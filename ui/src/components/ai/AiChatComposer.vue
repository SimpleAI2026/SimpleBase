<template>
  <div
    class="relative flex items-end gap-2 px-3 py-2.5"
    :class="disabled ? 'opacity-60' : ''"
    :style="{
      background: 'var(--composer-bg)',
      borderRadius: 'var(--composer-radius)',
      border: '1px solid var(--border)',
    }"
  >
    <Button variant="outline" size="icon-sm" class="rounded-full" disabled title="附件（即将支持）" aria-label="添加附件">
      <PlusIcon />
    </Button>
    <textarea
      ref="ta"
      class="max-h-55 min-h-6 min-w-0 flex-1 resize-none border-0 bg-transparent py-1 font-sans text-sm leading-normal text-foreground outline-none placeholder:text-muted-foreground"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled || sending"
      rows="1"
      @input="onInput"
      @keydown="onKeydown"
    />
    <Popover :open="mentionOpen && mentionAgents.length > 0">
      <PopoverTrigger as-child>
        <span class="sr-only">mention</span>
      </PopoverTrigger>
      <PopoverContent class="w-72 p-0" side="top" align="start">
        <Command>
          <CommandList>
            <CommandEmpty>无匹配 Agent</CommandEmpty>
            <CommandGroup>
              <CommandItem
                v-for="a in filteredMentions"
                :key="a.id"
                :value="a.name"
                @select="() => pickMention(a)"
              >
                <span class="font-semibold text-primary">@</span>{{ a.name }}
                <span class="ml-auto text-xs text-muted-foreground">{{ a.module }}</span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
    <Button variant="outline" size="icon-sm" class="rounded-full" disabled title="语音输入（即将支持）" aria-label="语音输入">
      <MicIcon />
    </Button>
    <Button
      v-if="sending"
      size="icon"
      class="rounded-full bg-destructive text-primary-foreground hover:bg-destructive/90"
      title="停止"
      aria-label="停止生成"
      @click="$emit('stop')"
    >
      <SquareIcon />
    </Button>
    <Button
      v-else
      size="icon"
      class="rounded-full"
      :style="{ background: 'var(--composer-send-bg)', color: 'var(--background)' }"
      :disabled="disabled || !modelValue.trim()"
      title="发送"
      aria-label="发送"
      @click="$emit('send', mentionsForSend(modelValue))"
    >
      <ArrowUpIcon />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ArrowUpIcon, MicIcon, PlusIcon, SquareIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
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
    placeholder: '输入消息，Enter 发送，Shift+Enter 换行；输入 @ 点名 Agent',
    disabled: false,
    sending: false,
    mentionAgents: () => []
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send', mentions: { agent_id: string }[]): void
  (e: 'stop'): void
}>()

const ta = ref<HTMLTextAreaElement | null>(null)
const mentionOpen = ref(false)
const mentionQuery = ref('')
const selected = ref<{ agent_id: string; name: string }[]>([])

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
  emit('update:modelValue', next)
  if (!selected.value.some((m) => m.agent_id === a.id)) {
    selected.value = [...selected.value, { agent_id: a.id, name: a.name }]
  }
  mentionOpen.value = false
  nextTick(() => ta.value?.focus())
}

function mentionsForSend(text: string) {
  const found: { agent_id: string }[] = []
  for (const a of props.mentionAgents) {
    if (text.includes('@' + a.name)) found.push({ agent_id: a.id })
  }
  for (const m of selected.value) {
    if (!found.some((x) => x.agent_id === m.agent_id)) found.push({ agent_id: m.agent_id })
  }
  return found
}

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
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
    if (!props.sending && props.modelValue.trim()) {
      emit('send', mentionsForSend(props.modelValue))
    }
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
  () => autosize()
)
</script>
