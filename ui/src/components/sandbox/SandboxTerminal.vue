<template>
  <div class="flex min-w-0 max-w-full flex-col gap-2">
    <SbEmptyState v-if="unavailable" :icon="TerminalIcon" :title="unavailableTitle" :description="unavailableDescription" />
    <template v-else>
      <div class="flex min-w-0 items-center justify-between gap-2 text-xs text-muted-foreground">
        <span data-testid="terminal-status" class="truncate">{{ statusText }}</span>
        <Button v-if="status === 'closed' || status === 'error'" size="sm" variant="outline" @click="connect">
          {{ t('components.sandboxTerminal.reconnect') }}
        </Button>
      </div>
      <div
        ref="host"
        data-testid="terminal-host"
        class="h-[min(60vh,28rem)] w-full min-w-0 max-w-full overflow-hidden rounded-md border bg-black p-2"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
/**
 * SandboxTerminal：xterm.js 终端，经一次性票据 + WebSocket 连到后端的 SSH 桥（planv6.2 §6.5）。
 *
 * - 二进制帧是终端数据，文本帧是 JSON 控制（resize / exit / error）。
 * - 断线后不自动重连，由用户点「重新连接」。
 * - xterm 动态 import，页签不打开就不加载。
 */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { TerminalIcon } from '@lucide/vue'
import type { FitAddon } from '@xterm/addon-fit'
import type { Terminal } from '@xterm/xterm'
import { Button } from '@/components/ui/button'
import SbEmptyState from '@/components/SbEmptyState.vue'
import { api } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import type { SandboxCapabilities, SandboxItem } from '../../services/types'

export type TerminalStatus = 'idle' | 'connecting' | 'connected' | 'closed' | 'error'

const props = defineProps<{
  projectId: string
  sandbox: SandboxItem
  capabilities: SandboxCapabilities | null
  /** 所在页签是否可见；隐藏时不建连，显示时 fit 并按需连接 */
  active: boolean
}>()

const { t } = useI18n()
const auth = useAuthStore()

const host = ref<HTMLElement | null>(null)
const status = ref<TerminalStatus>('idle')
const detail = ref('')

let term: Terminal | null = null
let fit: FitAddon | null = null
let socket: WebSocket | null = null
let observer: ResizeObserver | null = null
let generation = 0
const encoder = new TextEncoder()

/** 不可用的原因；按计划 §6.6 的四种空态排序。 */
const unavailableReason = computed<'agent' | 'unsupported' | 'notRunning' | 'readonly' | ''>(() => {
  if (props.sandbox.source === 'agent') return 'agent'
  if (!props.capabilities?.terminal) return 'unsupported'
  if (props.sandbox.state !== 'running') return 'notRunning'
  if (!auth.canWrite) return 'readonly'
  return ''
})
const unavailable = computed(() => unavailableReason.value !== '')
const unavailableTitle = computed(() => t('components.sandboxTerminal.unavailable.' + unavailableReason.value + 'Title'))
const unavailableDescription = computed(() => t('components.sandboxTerminal.unavailable.' + unavailableReason.value))

const statusText = computed(() => {
  const base = t('components.sandboxTerminal.status.' + status.value)
  return detail.value ? base + ' · ' + detail.value : base
})

async function ensureTerminal() {
  if (term || !host.value) return
  const [{ Terminal }, { FitAddon }] = await Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')])
  await import('@xterm/xterm/css/xterm.css')
  if (term || !host.value) return
  term = new Terminal({ cursorBlink: true, fontSize: 13, scrollback: 2000, convertEol: false })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host.value)
  term.onData((text) => {
    if (socket?.readyState === WebSocket.OPEN) socket.send(encoder.encode(text))
  })
  term.onResize(({ cols, rows }) => sendResize(cols, rows))
  observer = new ResizeObserver(() => doFit())
  observer.observe(host.value)
}

function doFit() {
  if (!props.active || !fit || !host.value || host.value.clientWidth === 0) return
  try {
    fit.fit()
  } catch {
    /* 容器尚未布局时 fit 会抛错，下一次 resize 再试 */
  }
}

function sendResize(cols: number, rows: number) {
  if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'resize', cols, rows }))
}

function closeSocket() {
  if (!socket) return
  const s = socket
  socket = null
  s.onopen = s.onmessage = s.onerror = s.onclose = null
  try {
    s.close()
  } catch {
    /* 已关闭 */
  }
}

async function connect() {
  const mine = ++generation
  closeSocket()
  detail.value = ''
  status.value = 'connecting'
  try {
    await ensureTerminal()
    if (mine !== generation) return
    term?.reset()
    const { ticket } = await api.sandboxes.terminalTicket(props.projectId, props.sandbox.id)
    if (mine !== generation) return
    const ws = new WebSocket(api.sandboxes.terminalWebSocketUrl(props.sandbox.id, ticket))
    ws.binaryType = 'arraybuffer'
    socket = ws
    ws.onopen = () => {
      if (mine !== generation) return
      status.value = 'connected'
      doFit()
      if (term) sendResize(term.cols, term.rows)
      term?.focus()
    }
    ws.onmessage = (event: MessageEvent) => {
      if (mine !== generation) return
      if (typeof event.data === 'string') {
        onControl(event.data)
        return
      }
      term?.write(new Uint8Array(event.data as ArrayBuffer))
    }
    ws.onerror = () => {
      if (mine !== generation) return
      status.value = 'error'
    }
    ws.onclose = () => {
      if (mine !== generation) return
      if (status.value !== 'error') status.value = 'closed'
    }
  } catch {
    if (mine !== generation) return
    status.value = 'error'
    detail.value = t('components.sandboxTerminal.ticketFailed')
  }
}

function onControl(text: string) {
  let ctl: { type?: string; code?: number; msg?: string }
  try {
    ctl = JSON.parse(text)
  } catch {
    return
  }
  if (ctl.type === 'exit') {
    status.value = 'closed'
    detail.value = t('components.sandboxTerminal.exited', { code: ctl.code ?? 0 })
  } else if (ctl.type === 'error') {
    status.value = 'error'
    detail.value = ctl.msg || ''
  }
}

function teardown() {
  generation++
  closeSocket()
  observer?.disconnect()
  observer = null
  term?.dispose()
  term = null
  fit = null
}

watch(
  () => [props.active, props.sandbox.id, unavailable.value] as const,
  async ([active, , blocked], prev) => {
    if (!active || blocked) {
      if (!active) generation++
      return
    }
    await nextTick()
    if (prev && prev[1] !== props.sandbox.id) {
      teardown()
    }
    if (!term) {
      await connect()
      return
    }
    doFit()
    if (status.value === 'idle') await connect()
  },
  { immediate: true, flush: 'post' }
)

onBeforeUnmount(teardown)

defineExpose({ connect, status })
</script>
