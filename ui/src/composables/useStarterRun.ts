import { computed, ref } from 'vue'
import { t } from '@/locales'
import {
  STARTER_STAGES,
  stagesFor,
  stageMessageKey,
  type StarterStage,
  type StarterTemplate
} from '@/constants/starter-prompts'
import type { AgentToolCallCard } from '@/services/types'
import { stageDoneByCards } from '@/utils/starter-progress'
import { parseDelivery, type StarterDelivery } from '@/utils/starter-delivery'

/** 单阶段的运行结果，由页面在 run 结束时回报。 */
export interface StageOutcome {
  /** run 是否正常结束（非 error / canceled）。 */
  ok: boolean
  /** 助手最终消息正文（plan 阶段用「非空」判定，verify 阶段解析交付块）。 */
  content: string
  /** 本轮全部工具卡片。 */
  cards: AgentToolCallCard[]
}

export interface StarterRunDeps {
  /**
   * 发送一轮阶段消息。实现方负责调用 conv.start 并带上 `starter_stage`，
   * run 结束后必须回调 `done`（含失败、取消）。
   */
  send: (text: string, stage: StarterStage, done: (outcome: StageOutcome) => void) => void
}

export type StageStatus = 'pending' | 'running' | 'done' | 'skipped' | 'failed'

/**
 * useStarterRun：分阶段 run 编排（planv6.6 home-starter-prompts §2.7）。
 *
 * 前端按阶段驱动多轮 run，不放宽全局 `agent_max_iterations` / `agent_run_timeout`：
 * plan → data → func → cron → site → verify。上一阶段成功才发下一阶段；任一阶段失败停在原处，
 * 由用户选「重试本阶段 / 跳过 / 我来接手」。用户手动发消息即退出自动模式（`abandon`）。
 *
 * 阶段完成判定来自真实的工具调用结果（`stageDoneByCards`），不依赖模型自报。
 * 一个 thread 同时只有一个 run（后端 ClaimThread 保证），阶段天然串行。
 */
export function useStarterRun(deps: StarterRunDeps) {
  const template = ref<StarterTemplate | null>(null)
  const stages = ref<StarterStage[]>([])
  const statuses = ref<Record<string, StageStatus>>({})
  const current = ref<StarterStage | null>(null)
  const active = ref(false)
  const stopped = ref(false)
  const delivery = ref<StarterDelivery | null>(null)
  const failure = ref<{ stage: StarterStage; reason: string } | null>(null)
  /** 当前阶段的发送序号：丢弃已被放弃 / 重试覆盖的旧回调。 */
  let epoch = 0

  const finished = computed(() => active.value && stages.value.length > 0 && stages.value.every((s) => statuses.value[s] === 'done' || statuses.value[s] === 'skipped'))
  const failedStage = computed(() => failure.value?.stage ?? null)

  function statusOf(stage: StarterStage): StageStatus {
    return statuses.value[stage] ?? 'pending'
  }

  /** 进度条分段：始终展示全部 6 段，被跳过的显示为 skipped。 */
  const progress = computed(() =>
    STARTER_STAGES.map((s) => ({ stage: s, status: stages.value.includes(s) ? statusOf(s) : ('skipped' as StageStatus) }))
  )

  function begin(tpl: StarterTemplate, siteReady: boolean) {
    epoch++
    template.value = tpl
    stages.value = stagesFor(tpl, siteReady)
    statuses.value = {}
    delivery.value = null
    failure.value = null
    stopped.value = false
    active.value = true
    current.value = null
  }

  function stageText(stage: StarterStage): string {
    return t(stageMessageKey(stage))
  }

  function runStage(stage: StarterStage) {
    const my = ++epoch
    current.value = stage
    failure.value = null
    statuses.value = { ...statuses.value, [stage]: 'running' }
    deps.send(stageText(stage), stage, (outcome) => {
      if (my !== epoch || !active.value) return
      settle(stage, outcome)
    })
  }

  function settle(stage: StarterStage, outcome: StageOutcome) {
    if (!outcome.ok) return fail(stage, t('components.starter.failed.runError'))
    const passed = judge(stage, outcome)
    if (!passed) return fail(stage, t(`components.starter.failed.${stage}`))
    statuses.value = { ...statuses.value, [stage]: 'done' }
    next(stage)
  }

  /** 阶段是否通过：工具调用为准；plan 看消息非空；verify 看交付块。 */
  function judge(stage: StarterStage, outcome: StageOutcome): boolean {
    switch (stage) {
      case 'plan':
        return outcome.content.trim() !== ''
      case 'verify': {
        const d = parseDelivery(outcome.content)
        if (d) delivery.value = d
        // 交付块缺失不算失败：退化为普通消息（计划 §2.8），只要校验类命令跑过即可。
        return d !== null || stageDoneByCards('verify', outcome.cards) || outcome.content.trim() !== ''
      }
      default:
        return stageDoneByCards(stage, outcome.cards)
    }
  }

  function next(after: StarterStage) {
    if (stopped.value) return
    const idx = stages.value.indexOf(after)
    const nxt = stages.value[idx + 1]
    if (!nxt) {
      current.value = null
      return
    }
    runStage(nxt)
  }

  function fail(stage: StarterStage, reason: string) {
    statuses.value = { ...statuses.value, [stage]: 'failed' }
    failure.value = { stage, reason }
    current.value = stage
  }

  /** 开始自动搭建：从第一阶段起。 */
  function start(tpl: StarterTemplate, siteReady: boolean) {
    begin(tpl, siteReady)
    if (stages.value.length) runStage(stages.value[0])
  }

  /** 重试失败阶段。 */
  function retry() {
    const f = failure.value
    if (!f || !active.value) return
    runStage(f.stage)
  }

  /** 跳过失败阶段，继续下一阶段。 */
  function skip() {
    const f = failure.value
    if (!f || !active.value) return
    statuses.value = { ...statuses.value, [f.stage]: 'skipped' }
    failure.value = null
    next(f.stage)
  }

  /** 我来接手 / 用户手动发消息：退出自动模式，保留已完成状态。 */
  function abandon() {
    epoch++
    stopped.value = true
    active.value = false
    failure.value = null
    current.value = null
  }

  return { template, stages, progress, current, active, finished, failure, failedStage, delivery, start, retry, skip, abandon, statusOf }
}
