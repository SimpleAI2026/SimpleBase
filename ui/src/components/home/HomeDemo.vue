<template>
  <section
    ref="root"
    class="flex flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-[0_12px_32px_rgba(32,41,35,0.08)] dark:shadow-none"
    data-home-demo
    :data-state="state"
    :aria-label="t('pages.home.demo.aria')"
  >
    <!-- 同一个订单应用连续经历「想法 → 本地 → 云端」（planv6.5 humanist §12.6）。
         进入视口后循环自动播放：云端结果停留 CLOUD_HOLD_MS 后从「想法」重新开始；暂停 / 重播是真实按钮。
         不做悬停暂停：鼠标停在演示上或点按钮时 mouseleave 不来（触屏上永远不来），会让演示「卡住」。
         系统开启「减少动态」时仍按拍推进内容，只去掉位移与过渡（motion-reduce:*）。 -->
    <header class="flex h-12 items-center justify-between gap-3 border-b border-border px-5">
      <span class="truncate text-sm font-medium text-foreground">{{ t('pages.home.demo.appName') }}</span>
      <!-- 签名细节「交接线」：本地 ─── 云端，只在这里出现 -->
      <span class="flex shrink-0 items-center gap-2 text-xs" data-handoff>
        <span class="inline-flex items-center gap-1.5" :class="env === 'local' ? 'text-foreground' : 'text-muted-foreground'">
          <span aria-hidden="true" class="size-1.5 rounded-full" :class="env === 'local' ? 'bg-primary' : 'border border-control-border'" />
          {{ t('pages.home.demo.local') }}
        </span>
        <span aria-hidden="true" class="relative h-px w-10 bg-border sm:w-16">
          <span
            class="absolute inset-0 origin-left bg-primary transition-transform duration-[420ms] motion-reduce:transition-none"
            :style="{ transform: `scaleX(${env === 'cloud' ? 1 : 0})` }"
          />
        </span>
        <span class="inline-flex items-center gap-1.5" :class="env === 'cloud' ? 'text-foreground' : 'text-muted-foreground'">
          <span aria-hidden="true" class="size-1.5 rounded-full" :class="env === 'cloud' ? 'bg-primary' : 'border border-control-border'" />
          {{ t('pages.home.demo.cloud') }}
        </span>
      </span>
    </header>

    <div class="grid flex-1 grid-cols-1 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]">
      <!-- 左：应用本身（主角，≥ 60%）。数据位置三阶段不变，只是逐步「建成」 -->
      <div class="flex min-w-0 flex-col gap-4 p-5" data-app>
        <div v-if="stage === 0" class="rounded-lg bg-muted px-3 py-2 text-sm text-foreground" data-ask>
          {{ t('pages.home.demo.chatAsk') }}
        </div>
        <ul v-else-if="stage === 1" class="sb-mono m-0 list-none rounded-lg bg-muted p-3 text-xs leading-6 text-text-secondary" data-workspace>
          <li v-for="f in files" :key="f.path" :style="{ paddingLeft: `${f.depth * 12}px` }">{{ f.path }}</li>
        </ul>

        <div class="flex flex-col">
          <div class="flex items-center justify-between border-b border-border pb-2 text-xs text-muted-foreground">
            <span>{{ t('pages.home.demo.ordersTitle') }}</span>
            <span class="inline-flex items-center gap-1.5">
              <CheckIcon v-if="parts.table" aria-hidden="true" class="size-3.5 text-success" />
              {{ parts.table ? t('pages.home.demo.partTableReady') : t('pages.home.demo.partTablePending') }}
            </span>
          </div>
          <div
            v-for="(o, oi) in orders"
            :key="o.id"
            class="sb-mono grid h-9 grid-cols-[4.5rem_minmax(0,1fr)_auto] items-center gap-3 border-b border-border text-xs transition-opacity duration-300 motion-reduce:transition-none"
            :class="parts.table ? 'opacity-100' : 'opacity-30'"
            :style="{ transitionDelay: parts.table ? `${oi * 50}ms` : '0ms' }"
          >
            <span class="text-text-secondary">#{{ o.id }}</span>
            <span class="tabular-nums text-foreground">{{ o.amount }}</span>
            <span class="text-text-secondary">{{ o.status }}</span>
          </div>
        </div>

        <div class="mt-auto grid grid-cols-2 gap-3">
          <div class="flex flex-col gap-1 rounded-lg border border-border p-3 transition-opacity duration-300 motion-reduce:transition-none" :class="parts.fn ? 'opacity-100' : 'opacity-40'">
            <span class="text-xs text-muted-foreground">{{ t('pages.home.demo.summaryLabel') }}</span>
            <span class="text-xl leading-none font-medium tabular-nums text-foreground">{{ parts.fn ? '¥8,420' : '—' }}</span>
          </div>
          <div class="flex flex-col gap-1 rounded-lg border border-border p-3 transition-opacity duration-300 motion-reduce:transition-none" :class="parts.cron ? 'opacity-100' : 'opacity-40'">
            <span class="text-xs text-muted-foreground">{{ t('pages.home.demo.cronLabel') }}</span>
            <span class="inline-flex items-center gap-1.5 text-sm text-foreground">
              <CheckIcon v-if="parts.cron" aria-hidden="true" class="size-3.5 text-success" />
              {{ t('pages.home.demo.cronValue') }}
            </span>
          </div>
        </div>
      </div>

      <!-- 右：证据栏（命令 / 输出，≤ 40%） -->
      <div class="flex min-w-0 flex-col gap-1 border-t border-border bg-background/60 p-5 sm:border-t-0 sm:border-l" data-evidence>
        <span class="mb-2 text-xs text-muted-foreground">{{ stage === 0 ? t('pages.home.demo.evidenceTools') : t('pages.home.demo.evidenceApply') }}</span>
        <template v-if="stage === 0">
          <p
            v-for="(c, ci) in toolCalls"
            :key="c.name"
            class="sb-mono m-0 flex items-center gap-2 text-xs leading-6 transition-[opacity,transform] duration-[260ms] motion-reduce:transition-none"
            :class="beat > ci ? 'translate-y-0 opacity-100' : 'translate-y-1 opacity-0 motion-reduce:translate-y-0'"
          >
            <CheckIcon aria-hidden="true" class="size-3.5 shrink-0 text-success" />
            <span class="truncate text-foreground">{{ c.name }}</span>
          </p>
        </template>
        <template v-else>
          <p class="sb-mono m-0 mb-1 text-xs leading-5 text-foreground">
            <span class="text-muted-foreground">$ </span>simplebase dev apply <span class="whitespace-nowrap">--dir .</span> <span class="whitespace-nowrap">--yes</span>
          </p>
          <p
            v-for="(a, ai) in applyActions"
            :key="a.name"
            class="sb-mono m-0 grid grid-cols-[3.5rem_minmax(0,1fr)] text-xs leading-6 transition-[opacity,transform] duration-[260ms] motion-reduce:transition-none"
            :class="stage === 2 || beat > ai ? 'translate-y-0 opacity-100' : 'translate-y-1 opacity-0 motion-reduce:translate-y-0'"
          >
            <span class="text-success">{{ a.verb }}</span>
            <span class="truncate text-text-secondary">{{ a.name }}</span>
          </p>
        </template>
      </div>
    </div>

    <!-- 云端阶段：结果 + 共享存储底座 + 紧邻的成本前提 -->
    <div v-if="stage === 2" class="flex flex-col gap-3 border-t border-border px-5 py-4" data-foundation>
      <p class="m-0 inline-flex items-center gap-2 text-sm text-foreground">
        <CheckIcon aria-hidden="true" class="size-4 text-success" />{{ t('pages.home.demo.shipped', { n: applyActions.length }) }}
      </p>
      <p class="m-0 text-xs text-text-secondary">{{ t('pages.home.demo.foundationNote') }}</p>
    </div>

    <footer class="flex flex-wrap items-center justify-between gap-3 border-t border-border px-5 py-3">
      <ol class="m-0 flex list-none items-center gap-1 p-0" :aria-label="t('pages.home.demo.stepsAria')">
        <li v-for="(s, i) in stages" :key="s" class="flex items-center gap-1">
          <button
            type="button"
            class="inline-flex h-11 items-center gap-1.5 rounded-lg px-2 text-xs transition-colors hover:bg-muted sm:h-8"
            :class="i === stage ? 'text-foreground' : 'text-muted-foreground'"
            :aria-current="i === stage ? 'step' : undefined"
            :data-stage="i"
            @click="go(i)"
          >
            <span aria-hidden="true" class="size-1.5 rounded-full" :class="i <= stage ? 'bg-primary' : 'border border-control-border'" />{{ s }}
          </button>
          <span v-if="i < stages.length - 1" aria-hidden="true" class="h-px w-3 bg-border" />
        </li>
      </ol>
      <div class="flex items-center gap-1">
        <span class="mr-2 text-xs text-muted-foreground">{{ t('pages.home.demo.sampleTag') }}</span>
        <button
          type="button"
          class="inline-flex size-11 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:size-8"
          :aria-label="manualPaused ? t('pages.home.demo.resume') : t('pages.home.demo.pause')"
          data-demo-toggle
          @click="togglePause"
        >
          <PlayIcon v-if="manualPaused" aria-hidden="true" class="size-4" />
          <PauseIcon v-else aria-hidden="true" class="size-4" />
        </button>
        <button
          type="button"
          class="inline-flex size-11 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:size-8"
          :aria-label="t('pages.home.demo.replay')"
          data-demo-replay
          @click="replay"
        >
          <RotateCcwIcon aria-hidden="true" class="size-4" />
        </button>
      </div>
    </footer>
    <p class="sr-only" aria-live="polite">{{ stage === 2 ? t('pages.home.demo.shipped', { n: applyActions.length }) : '' }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckIcon, PauseIcon, PlayIcon, RotateCcwIcon } from '@lucide/vue'

const { t } = useI18n()

/** 每阶段的拍数与节奏：拍间 600ms，阶段末停留 1400ms；云端结果多停留一会儿再从头循环。 */
const STAGE_BEATS = [3, 4, 1] as const
const BEAT_MS = 600
const HOLD_MS = 1400
const CLOUD_HOLD_MS = 5000

const stage = ref(0)
const beat = ref(0)
const manualPaused = ref(false)
const visible = ref(false)
const pageHidden = ref(false)
const root = ref<HTMLElement | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
let observer: IntersectionObserver | null = null

const stages = computed(() => [t('pages.home.demo.stage1'), t('pages.home.demo.stage2'), t('pages.home.demo.stage3')])

// 命令、文件名、单号是代码字面量，不进语言包；金额只是界面示例（已标注「交互演示」）。
const toolCalls = [{ name: 'sql exec · orders' }, { name: 'function create · daily_summary' }, { name: 'cron create · nightly' }]
const files = [
  { path: 'simplebase.json', depth: 0 },
  { path: 'migrations/', depth: 0 },
  { path: '001_orders.sql', depth: 1 },
  { path: 'functions/', depth: 0 },
  { path: 'daily_summary.go', depth: 1 }
]
const applyActions = [
  { verb: 'create', name: 'database shop' },
  { verb: 'apply', name: '001_orders.sql' },
  { verb: 'create', name: 'daily_summary' },
  { verb: 'create', name: 'cron nightly' }
]
const orders = computed(() => [
  { id: 1042, amount: '¥128.00', status: t('pages.home.demo.statusPaid') },
  { id: 1041, amount: '¥56.00', status: t('pages.home.demo.statusShipping') },
  { id: 1040, amount: '¥312.00', status: t('pages.home.demo.statusPaid') }
])

/** 想法阶段逐个建成；进入本地 / 云端阶段后三件都已存在。 */
const parts = computed(() => ({
  table: stage.value > 0 || beat.value >= 1,
  fn: stage.value > 0 || beat.value >= 2,
  cron: stage.value > 0 || beat.value >= 3
}))
const env = computed(() => (stage.value === 2 ? 'cloud' : 'local'))
const running = computed(() => !manualPaused.value && visible.value && !pageHidden.value)
/** playing：正在播；paused：用户按了暂停；idle：不在视口或页面在后台，回来后自动继续。 */
const state = computed(() => (manualPaused.value ? 'paused' : running.value ? 'playing' : 'idle'))

function clear() {
  if (timer) clearTimeout(timer)
  timer = undefined
}

function tick() {
  timer = undefined
  // 定时器到点时状态可能已变（切后台、滚出视口），不在播放就不推进，等 running 变回 true 再排
  if (!running.value) return
  if (beat.value < STAGE_BEATS[stage.value]) {
    beat.value += 1
  } else {
    stage.value = (stage.value + 1) % STAGE_BEATS.length
    beat.value = 0
  }
  schedule()
}

/** 只保留一个定时器；每次都先清掉旧的，避免多次触发叠加或丢失 */
function schedule() {
  clear()
  if (!running.value) return
  const lastBeat = beat.value >= STAGE_BEATS[stage.value]
  const isCloud = stage.value === STAGE_BEATS.length - 1
  timer = setTimeout(tick, lastBeat ? (isCloud ? CLOUD_HOLD_MS : HOLD_MS) : BEAT_MS)
}

/** 手动跳到某阶段：从该阶段开头播放（暂停中则保持暂停，只展示该阶段完整状态）。 */
function go(i: number) {
  stage.value = i
  beat.value = manualPaused.value ? STAGE_BEATS[i] : 0
  schedule()
}

function replay() {
  stage.value = 0
  beat.value = 0
  manualPaused.value = false
  schedule()
}

function togglePause() {
  manualPaused.value = !manualPaused.value
}

function onVisibility() {
  pageHidden.value = document.visibilityState === 'hidden'
}

watch(running, (r) => (r ? schedule() : clear()))

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibility)
  // 进入视口才开始；无 IntersectionObserver 的环境（测试、旧浏览器）直接视为可见
  if (typeof IntersectionObserver === 'undefined' || !root.value) {
    visible.value = true
  } else {
    // 阈值取 0.2：演示在矮屏上比视口还高时，0.35 可能永远达不到，导致一直不播
    observer = new IntersectionObserver((entries) => {
      visible.value = entries.some((e) => e.isIntersecting)
    }, { threshold: 0.2 })
    observer.observe(root.value)
  }
})

onBeforeUnmount(() => {
  clear()
  observer?.disconnect()
  document.removeEventListener('visibilitychange', onVisibility)
})

defineExpose({ stage, beat, state, go, replay })
</script>
