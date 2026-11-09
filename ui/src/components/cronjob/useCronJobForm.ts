/**
 * 定时任务表单状态与校验（从 CronJobModal 拆出）。
 *
 * 沿用 plan/planv5.0/ui-optimization-v3-plan.md §3.6 的拆分：模板职责归组件，
 * 表单状态/校验/提交归 composable，便于单独测试且让宿主模板只做编排。
 *
 * 注意：`CronJobModal.vue` 会用 `<script setup>` 顶层绑定直接暴露这里的 ref，
 * 既有测试通过 `wrapper.vm.form` / `vm.intervalUnit` 访问，拆分不得破坏该表面。
 */
import { computed, reactive, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useLoadState } from '@/composables/useLoadState'
import { errorMessage } from '@/utils/format'
import {
  api,
  CRON_AGENT_PROMPT_MAX_BYTES,
  type CloudAgent,
  type CronJobItem,
  type CronTargetKind,
  type GoFunctionItem
} from '@/services/api'

export type ScheduleKind = 'cron' | 'interval' | 'once'
export type IntervalUnit = 'minute' | 'hour' | 'day'

/** 任务名格式（对齐后端 cronJobNameRe）。 */
const CRON_NAME_RE = /^[A-Za-z][A-Za-z0-9_-]{0,62}$/

const textEncoder = new TextEncoder()

/** 提示词的 UTF-8 字节数：后端按 Go len()（字节）校验 16KiB，前端同口径。 */
export function promptBytes(s: string): number {
  return textEncoder.encode(s).length
}

export interface CronJobForm {
  name: string
  description: string
  targetKind: CronTargetKind
  scheduleKind: ScheduleKind
  cronExpr: string
  runAtLocal: string
  funcFile: string
  funcExport: string
  inputJson: string
  agentId: string
  agentPrompt: string
}

export interface UseCronJobFormOptions {
  /** 编辑目标；undefined = 新建 */
  target: Ref<CronJobItem | undefined>
  projectId: Ref<string>
  /** 弹窗打开状态，打开时初始化表单 */
  open: Ref<boolean>
  /** 新建时预选的助手（AgentManager 跳转而来）；有值则默认切到「助手」类型 */
  presetAgentId?: Ref<string | undefined>
  /** 保存成功后回调（宿主负责 emit saved + 关窗） */
  onSaved: () => void
}

export interface UseCronJobForm {
  form: CronJobForm
  isEdit: ComputedRef<boolean>
  intervalValue: Ref<number>
  intervalUnit: Ref<IntervalUnit>
  preset: Ref<string | undefined>
  intervalSeconds: ComputedRef<number>
  gofunctions: Ref<GoFunctionItem[]>
  agents: Ref<CloudAgent[]>
  agentsPending: Ref<boolean>
  agentsFailed: Ref<boolean>
  agentPromptBytes: ComputedRef<number>
  agentPromptError: ComputedRef<string>
  loadAgents: () => Promise<void>
  pending: ReturnType<typeof useLoadState>['pending']
  settled: ReturnType<typeof useLoadState>['settled']
  showError: ReturnType<typeof useLoadState>['showError']
  error: ReturnType<typeof useLoadState>['error']
  currentExports: ComputedRef<string[]>
  availableFunctions: ComputedRef<GoFunctionItem[]>
  unpublishedCount: ComputedRef<number>
  inputJsonError: ComputedRef<string>
  nameError: ComputedRef<string>
  canSave: ComputedRef<boolean>
  saving: Ref<boolean>
  loadFunctions: () => Promise<void>
  save: () => Promise<void>
}

/** `presets` 里的 label 是 i18n key，渲染处再翻译。 */
export const CRON_PRESETS: { labelKey: string; expr: string }[] = [
  { labelKey: 'components.cronJobModal.presetHourly', expr: '0 * * * *' },
  { labelKey: 'components.cronJobModal.presetDaily2', expr: '0 2 * * *' },
  { labelKey: 'components.cronJobModal.presetWeeklyMon9', expr: '0 9 * * 1' },
  { labelKey: 'components.cronJobModal.presetMonthly1', expr: '0 0 1 * *' }
]

/** datetime-local（本地展示）↔ ISO UTC */
export function toLocalInput(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function toRunAtISO(local: string): string {
  if (!local) return ''
  const d = new Date(local)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}

export function useCronJobForm(options: UseCronJobFormOptions): UseCronJobForm {
  const { t } = useI18n()

  const form = reactive<CronJobForm>({
    name: '',
    description: '',
    targetKind: 'function',
    scheduleKind: 'cron',
    cronExpr: '0 2 * * *',
    runAtLocal: '',
    funcFile: '',
    funcExport: '',
    inputJson: '',
    agentId: '',
    agentPrompt: ''
  })

  const isEdit = computed(() => Boolean(options.target.value))
  const intervalValue = ref(10)
  const intervalUnit = ref<IntervalUnit>('minute')
  const preset = ref<string>()

  watch(preset, (expr) => {
    if (expr) form.cronExpr = expr
  })

  /** interval 秒数换算 */
  const intervalSeconds = computed(() => {
    const v = intervalValue.value
    if (!Number.isFinite(v) || v <= 0) return 0
    switch (intervalUnit.value) {
      case 'day':
        return Math.round(v) * 86400
      case 'hour':
        return Math.round(v) * 3600
      default:
        return Math.round(v) * 60
    }
  })

  /** 提交用调度字段 */
  function schedulePayload() {
    return {
      scheduleKind: form.scheduleKind,
      cronExpr: form.scheduleKind === 'cron' ? form.cronExpr.trim() : '',
      intervalSeconds: form.scheduleKind === 'interval' ? intervalSeconds.value : undefined,
      runAt: form.scheduleKind === 'once' ? toRunAtISO(form.runAtLocal) : undefined
    }
  }

  /* ---------- 云函数联动 ---------- */

  const gofunctions = ref<GoFunctionItem[]>([])
  const { pending, settled, showError, error, run } = useLoadState({
    fallback: 'components.cronJobModal.loadFunctionsFailed'
  })

  const currentExports = computed(() => {
    const g = gofunctions.value.find((x) => x.name === form.funcFile)
    return g ? g.exports : []
  })

  function loadFunctions() {
    const id = options.projectId.value
    return run(async () => {
      try {
        const data = await api.gofunctions.list(id)
        gofunctions.value = data
        return data.length > 0
      } catch (e) {
        gofunctions.value = []
        throw e
      }
    }, { replace: true })
  }

  /* ---------- 助手联动 ---------- */

  const agents = ref<CloudAgent[]>([])
  const agentsPending = ref(false)
  const agentsFailed = ref(false)

  async function loadAgents() {
    agentsPending.value = true
    agentsFailed.value = false
    try {
      agents.value = await api.agents.list(options.projectId.value)
    } catch {
      agents.value = []
      agentsFailed.value = true
    } finally {
      agentsPending.value = false
    }
  }

  /* ---------- 打开时初始化 ---------- */

  function resetForCreate() {
    form.name = ''
    form.description = ''
    form.targetKind = options.presetAgentId?.value ? 'agent' : 'function'
    form.scheduleKind = 'cron'
    form.cronExpr = '0 2 * * *'
    form.runAtLocal = ''
    form.funcFile = ''
    form.funcExport = ''
    form.inputJson = ''
    form.agentId = options.presetAgentId?.value ?? ''
    form.agentPrompt = ''
    intervalUnit.value = 'minute'
    intervalValue.value = 10
  }

  function initFromTarget() {
    const target = options.target.value
    if (!target) {
      resetForCreate()
      return
    }
    form.name = target.name
    form.description = target.description
    form.targetKind = target.targetKind ?? 'function'
    form.scheduleKind = target.scheduleKind
    form.cronExpr = target.cronExpr || '0 2 * * *'
    form.runAtLocal = target.runAt ? toLocalInput(target.runAt) : ''
    form.funcFile = target.funcFile
    form.funcExport = target.funcExport
    form.inputJson = target.inputJson
    form.agentId = target.agentId ?? ''
    form.agentPrompt = target.agentPrompt ?? ''
    if (target.scheduleKind === 'interval' && target.intervalSeconds) {
      const s = target.intervalSeconds
      if (s % 86400 === 0) {
        intervalUnit.value = 'day'
        intervalValue.value = s / 86400
      } else if (s % 3600 === 0) {
        intervalUnit.value = 'hour'
        intervalValue.value = s / 3600
      } else {
        intervalUnit.value = 'minute'
        intervalValue.value = Math.ceil(s / 60)
      }
    }
  }

  watch(
    () => options.open.value,
    (open) => {
      if (!open) return
      void loadFunctions()
      void loadAgents()
      initFromTarget()
      preset.value = undefined
    },
    { immediate: true }
  )

  /* ---------- 校验与保存 ---------- */

  const inputJsonError = computed(() => {
    const s = form.inputJson.trim()
    if (!s) return ''
    try {
      JSON.parse(s)
      return ''
    } catch {
      return t('components.cronJobModal.invalidJson')
    }
  })

  const nameError = computed(() => {
    if (isEdit.value) return ''
    const s = form.name.trim()
    if (!s) return ''
    if (!CRON_NAME_RE.test(s)) return t('components.cronJobModal.nameInvalid')
    return ''
  })

  /** 仅已发布（有生效版）的云函数可作定时任务目标（后端 validateTarget 同口径） */
  const availableFunctions = computed(() => {
    const list = gofunctions.value.filter((g) => g.published && g.activeVersion > 0)
    // 编辑时保留当前目标（即使已未发布），避免 Select 显示为空
    if (isEdit.value && form.funcFile && !list.some((g) => g.name === form.funcFile)) {
      const cur = gofunctions.value.find((g) => g.name === form.funcFile)
      if (cur) return [cur, ...list]
    }
    return list
  })

  const unpublishedCount = computed(
    () => gofunctions.value.filter((g) => !(g.published && g.activeVersion > 0)).length
  )

  const agentPromptBytes = computed(() => promptBytes(form.agentPrompt))

  const agentPromptError = computed(() =>
    agentPromptBytes.value > CRON_AGENT_PROMPT_MAX_BYTES
      ? t('components.cronJobModal.agentPromptTooLong', { max: CRON_AGENT_PROMPT_MAX_BYTES })
      : ''
  )

  const canSave = computed(() => {
    if (!form.name.trim() || nameError.value) return false
    if (form.scheduleKind === 'cron' && form.cronExpr.trim().split(/\s+/).length !== 5) return false
    if (form.scheduleKind === 'interval' && intervalSeconds.value < 60) return false
    if (form.scheduleKind === 'once' && !form.runAtLocal) return false
    if (form.targetKind === 'agent') {
      return Boolean(form.agentId) && Boolean(form.agentPrompt.trim()) && !agentPromptError.value
    }
    if (!form.funcFile || !form.funcExport) return false
    const target = gofunctions.value.find((g) => g.name === form.funcFile)
    if (!target || !(target.published && target.activeVersion > 0)) return false
    return !inputJsonError.value
  })

  /** 目标相关字段：只提交当前类型那一侧，另一侧不带，避免后端残留脏值。 */
  function targetPayload() {
    if (form.targetKind === 'agent') {
      return {
        targetKind: 'agent' as const,
        agentId: form.agentId,
        agentPrompt: form.agentPrompt
      }
    }
    return {
      targetKind: 'function' as const,
      funcFile: form.funcFile,
      funcExport: form.funcExport,
      inputJson: form.inputJson.trim() || '{}'
    }
  }

  const saving = ref(false)

  async function save() {
    if (!canSave.value || saving.value) return
    saving.value = true
    try {
      const target = options.target.value
      if (isEdit.value && target) {
        await api.cronjobs.update(options.projectId.value, target.id, {
          description: form.description,
          ...schedulePayload(),
          ...targetPayload(),
          enabled: target.enabled
        })
        toast.success(t('components.cronJobModal.updated', { name: form.name }))
      } else {
        const created = await api.cronjobs.create(options.projectId.value, {
          name: form.name.trim(),
          description: form.description,
          ...schedulePayload(),
          ...targetPayload()
        })
        if (created.nextRunAt) {
          toast.success(
            t('components.cronJobModal.createdWithNext', {
              name: form.name,
              time: new Date(created.nextRunAt).toLocaleString()
            })
          )
        } else {
          toast.success(t('components.cronJobModal.created', { name: form.name }))
        }
      }
      options.onSaved()
    } catch (e) {
      toast.error(errorMessage(e, t('components.cronJobModal.saveFailed')))
    } finally {
      saving.value = false
    }
  }

  return {
    form,
    isEdit,
    intervalValue,
    intervalUnit,
    preset,
    intervalSeconds,
    gofunctions,
    agents,
    agentsPending,
    agentsFailed,
    agentPromptBytes,
    agentPromptError,
    loadAgents,
    pending,
    settled,
    showError,
    error,
    currentExports,
    availableFunctions,
    unpublishedCount,
    inputJsonError,
    nameError,
    canSave,
    saving,
    loadFunctions,
    save
  }
}
