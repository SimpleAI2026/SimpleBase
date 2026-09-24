<template>
  <SbModal
    :open="open"
    :title="isEdit ? '编辑定时任务' : '新建定时任务'"
    :width="860"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !canSave }"
    @update:open="$emit('update:open', $event)"
    @ok="save"
  >
    <div class="flex max-h-[70vh] flex-col gap-4 overflow-y-auto py-2 pr-1">
      <FieldGroup>
        <Field :data-invalid="nameError ? true : undefined">
          <FieldLabel>任务名</FieldLabel>
          <Input
            v-model="form.name"
            class="sb-mono"
            :disabled="isEdit"
            placeholder="nightly-refresh"
            :aria-invalid="nameError ? true : undefined"
          />
          <FieldDescription>{{ isEdit ? '创建后不可修改' : '字母开头，可含字母数字 _ -' }}</FieldDescription>
          <FieldError v-if="nameError">{{ nameError }}</FieldError>
        </Field>
        <Field>
          <FieldLabel>描述</FieldLabel>
          <Input v-model="form.description" placeholder="每天凌晨汇总昨日数据" />
        </Field>
      </FieldGroup>

      <Separator />

      <FieldSet>
        <FieldLegend>时间执行</FieldLegend>
        <FieldDescription>三种模式任选其一；调度一律按 UTC 解释。</FieldDescription>
        <FieldContent>
          <div class="flex flex-wrap items-center gap-6 pt-1.5">
            <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
              <input
                v-model="form.scheduleKind"
                type="radio"
                name="schedule-kind"
                value="cron"
                class="size-4 shrink-0 accent-primary"
              />
              <span>定时执行（cron）</span>
            </label>
            <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
              <input
                v-model="form.scheduleKind"
                type="radio"
                name="schedule-kind"
                value="interval"
                class="size-4 shrink-0 accent-primary"
              />
              <span>固定间隔</span>
            </label>
            <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
              <input
                v-model="form.scheduleKind"
                type="radio"
                name="schedule-kind"
                value="once"
                class="size-4 shrink-0 accent-primary"
              />
              <span>一次性执行</span>
            </label>
          </div>
        </FieldContent>

        <Field v-if="form.scheduleKind === 'cron'" class="pt-3">
          <FieldLabel>cron 表达式</FieldLabel>
          <div class="flex gap-2">
            <Input v-model="form.cronExpr" class="sb-mono flex-1" placeholder="0 2 * * *" />
            <Select v-model="preset">
              <SelectTrigger class="w-36 shrink-0"><SelectValue placeholder="常用预设" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="p in presets" :key="p.expr" :value="p.expr">{{ p.label }}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <FieldDescription>分 时 日 月 周 · 示例 <code class="sb-mono">0 2 * * *</code> = 每天 02:00 UTC</FieldDescription>
        </Field>

        <Field v-else-if="form.scheduleKind === 'interval'" class="pt-3">
          <FieldLabel>执行间隔</FieldLabel>
          <div class="flex items-center gap-2">
            <span class="text-sm text-muted-foreground">每</span>
            <Input
              v-model.number="intervalValue"
              type="number"
              min="1"
              max="30"
              class="sb-mono w-24"
            />
            <Select v-model="intervalUnit">
              <SelectTrigger class="w-28 shrink-0"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="minute">分钟</SelectItem>
                <SelectItem value="hour">小时</SelectItem>
                <SelectItem value="day">天</SelectItem>
              </SelectContent>
            </Select>
            <span class="text-sm text-muted-foreground">执行一次</span>
          </div>
          <FieldDescription>范围 1 分钟 ~ 30 天</FieldDescription>
        </Field>

        <Field v-else class="pt-3">
          <FieldLabel>执行时刻</FieldLabel>
          <Input
            v-model="form.runAtLocal"
            type="datetime-local"
            class="sb-mono w-full sm:w-64"
          />
          <FieldDescription>到点执行一次后自动停用；时间按 UTC 解释</FieldDescription>
        </Field>
      </FieldSet>

      <Separator />

      <FieldGroup>
        <Field>
          <FieldLabel>云函数文件</FieldLabel>
          <Select v-model="form.funcFile" @update:model-value="form.funcExport = ''">
            <SelectTrigger>
              <SelectValue placeholder="选择文件" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="g in availableFunctions" :key="g.id" :value="g.name">
                {{ g.file }}
              </SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription v-if="!gofunctions.length">
            项目内还没有云函数，
            <RouterLink to="/gofunctions" class="text-primary underline">先去创建</RouterLink>
          </FieldDescription>
          <FieldDescription v-else-if="!availableFunctions.length">
            没有已发布的云函数（需有生效版本），
            <RouterLink to="/gofunctions" class="text-primary underline">先去发布</RouterLink>
          </FieldDescription>
          <FieldDescription v-else-if="unpublishedCount > 0">
            仅列出已发布（有生效版）的云函数；{{ unpublishedCount }} 个未发布不可选
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel>导出函数</FieldLabel>
          <Select v-model="form.funcExport">
            <SelectTrigger>
              <SelectValue placeholder="选择函数" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="fn in currentExports" :key="fn" :value="fn">{{ fn }}</SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription>仅列出该文件已导出的大写函数；修改云函数后按最新代码执行</FieldDescription>
        </Field>
      </FieldGroup>

      <Field :data-invalid="inputJsonError ? true : undefined">
        <FieldLabel>入参 JSON（可选）</FieldLabel>
        <Textarea
          v-model="form.inputJson"
          rows="3"
          class="sb-mono"
          :aria-invalid="inputJsonError ? true : undefined"
          placeholder='{ "name": "cron" }'
        />
        <FieldDescription>将在每次执行时作为请求 body 传入；留空为 {}</FieldDescription>
        <FieldError v-if="inputJsonError">{{ inputJsonError }}</FieldError>
      </Field>

      <Alert>
        <InfoIcon class="size-4" />
        <AlertDescription>
          任务到点执行超时上限 10 秒；入参须与函数签名匹配（struct 用 JSON 对象），不匹配将在执行时记为失败。
        </AlertDescription>
      </Alert>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 定时任务新建/编辑 Modal（ui-cronjob-plan §7.3）。
 * 文件→导出函数两级联动 Select；interval 用数值+单位换算秒；保存走 create/update。
 */
import { computed, reactive, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { toast } from 'vue-sonner'
import { InfoIcon } from '@lucide/vue'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import SbModal from './SbModal.vue'
import { api } from '../../services/api'
import type { CronJobItem, GoFunctionItem } from '../../services/api'
import { useProjectStore } from '../../stores/project'

const props = defineProps<{
  open: boolean
  /** 编辑目标；undefined = 新建 */
  target?: CronJobItem
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const projectStore = useProjectStore()
const projectId = computed(() => projectStore.projectId)

const isEdit = computed(() => Boolean(props.target))

/* ---------- 表单状态 ---------- */

const form = reactive({
  name: '',
  description: '',
  scheduleKind: 'cron' as 'cron' | 'interval' | 'once',
  cronExpr: '0 2 * * *',
  runAtLocal: '',
  funcFile: '',
  funcExport: '',
  inputJson: ''
})

const intervalValue = ref(10)
const intervalUnit = ref<'minute' | 'hour' | 'day'>('minute')
const preset = ref<string>()

const presets = [
  { label: '每小时', expr: '0 * * * *' },
  { label: '每天 02:00', expr: '0 2 * * *' },
  { label: '每周一 09:00', expr: '0 9 * * 1' },
  { label: '每月 1 日 00:00', expr: '0 0 1 * *' }
]

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

/** datetime-local（本地展示）↔ ISO UTC */
function toLocalInput(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function toRunAtISO(local: string): string {
  if (!local) return ''
  const d = new Date(local)
  return Number.isNaN(d.getTime()) ? '' : d.toISOString()
}

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
const currentExports = computed(() => {
  const g = gofunctions.value.find((x) => x.name === form.funcFile)
  return g ? g.exports : []
})

/** 换文件清空函数选择：由 Select 的 @update:model-value 驱动（避免初始化被异步 watch 误重置） */

async function loadFunctions() {
  try {
    gofunctions.value = await api.gofunctions.list(projectId.value)
  } catch {
    gofunctions.value = []
  }
}

/* ---------- 打开时初始化 ---------- */

watch(
  () => props.open,
  (open) => {
    if (!open) return
    loadFunctions()
    const t = props.target
    if (t) {
      form.name = t.name
      form.description = t.description
      form.scheduleKind = t.scheduleKind
      form.cronExpr = t.cronExpr || '0 2 * * *'
      form.runAtLocal = t.runAt ? toLocalInput(t.runAt) : ''
      form.funcFile = t.funcFile
      form.funcExport = t.funcExport
      form.inputJson = t.inputJson
      if (t.scheduleKind === 'interval' && t.intervalSeconds) {
        const s = t.intervalSeconds
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
    } else {
      form.name = ''
      form.description = ''
      form.scheduleKind = 'cron'
      form.cronExpr = '0 2 * * *'
      form.runAtLocal = ''
      form.funcFile = ''
      form.funcExport = ''
      form.inputJson = ''
      intervalUnit.value = 'minute'
      intervalValue.value = 10
    }
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
    return '入参不是合法 JSON'
  }
})

/** 任务名格式（对齐后端 cronJobNameRe） */
const nameError = computed(() => {
  if (isEdit.value) return ''
  const s = form.name.trim()
  if (!s) return ''
  if (!/^[A-Za-z][A-Za-z0-9_-]{0,62}$/.test(s)) return '字母开头，可含字母数字 _ -（1~63 位）'
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

const canSave = computed(() => {
  if (!form.name.trim() || nameError.value) return false
  if (form.scheduleKind === 'cron' && form.cronExpr.trim().split(/\s+/).length !== 5) return false
  if (form.scheduleKind === 'interval' && intervalSeconds.value < 60) return false
  if (form.scheduleKind === 'once' && !form.runAtLocal) return false
  if (!form.funcFile || !form.funcExport) return false
  const target = gofunctions.value.find((g) => g.name === form.funcFile)
  if (!target || !(target.published && target.activeVersion > 0)) return false
  return !inputJsonError.value
})

const saving = ref(false)

async function save() {
  if (!canSave.value || saving.value) return
  saving.value = true
  const inputJson = form.inputJson.trim() || '{}'
  try {
    if (isEdit.value && props.target) {
      await api.cronjobs.update(projectId.value, props.target.id, {
        description: form.description,
        ...schedulePayload(),
        funcFile: form.funcFile,
        funcExport: form.funcExport,
        inputJson,
        enabled: props.target.enabled
      })
      toast.success(`任务已更新：${form.name}`)
    } else {
      const created = await api.cronjobs.create(projectId.value, {
        name: form.name.trim(),
        description: form.description,
        ...schedulePayload(),
        funcFile: form.funcFile,
        funcExport: form.funcExport,
        inputJson
      })
      const next = created.nextRunAt ? `，下次执行 ${new Date(created.nextRunAt).toLocaleString()}` : ''
      toast.success(`任务已创建：${form.name}${next}`)
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}

/* SbModal footer 确认按钮由组件内部渲染；okButtonProps 控制禁用。 */
</script>
