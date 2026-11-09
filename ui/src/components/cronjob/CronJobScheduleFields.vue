<template>
  <FieldSet>
    <FieldLegend>{{ t('components.cronJobModal.scheduleLegend') }}</FieldLegend>
    <FieldDescription>{{ t('components.cronJobModal.scheduleHint') }}</FieldDescription>
    <FieldContent>
      <div class="flex flex-wrap items-center gap-6 pt-1.5">
        <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
          <input
            v-model="scheduleKind"
            type="radio"
            name="schedule-kind"
            value="cron"
            class="size-4 shrink-0 accent-primary"
          />
          <span>{{ t('components.cronJobModal.modeCron') }}</span>
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
          <input
            v-model="scheduleKind"
            type="radio"
            name="schedule-kind"
            value="interval"
            class="size-4 shrink-0 accent-primary"
          />
          <span>{{ t('components.cronJobModal.modeInterval') }}</span>
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
          <input
            v-model="scheduleKind"
            type="radio"
            name="schedule-kind"
            value="once"
            class="size-4 shrink-0 accent-primary"
          />
          <span>{{ t('components.cronJobModal.modeOnce') }}</span>
        </label>
      </div>
    </FieldContent>

    <Field v-if="scheduleKind === 'cron'" class="pt-3">
      <FieldLabel for="cron-expression">{{ t('components.cronJobModal.cronExpression') }}</FieldLabel>
      <div class="flex min-w-0 flex-col gap-2 sm:flex-row">
        <Input id="cron-expression" v-model="cronExpr" class="sb-mono min-w-0 flex-1" placeholder="0 2 * * *" aria-describedby="cron-expression-help" />
        <Select v-model="presetValue">
          <SelectTrigger class="w-full sm:w-36 sm:shrink-0" :aria-label="t('components.cronJobModal.cronPresetAria')"><SelectValue :placeholder="t('components.cronJobModal.cronPresetPlaceholder')" /></SelectTrigger>
          <SelectContent>
            <SelectItem v-for="p in presets" :key="p.expr" :value="p.expr">{{ t(p.labelKey) }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <FieldDescription id="cron-expression-help">
        {{ t('components.cronJobModal.cronExpressionHint') }}
      </FieldDescription>
    </Field>

    <Field v-else-if="scheduleKind === 'interval'" class="pt-3">
      <FieldLabel for="cron-interval-value">{{ t('components.cronJobModal.intervalLabel') }}</FieldLabel>
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-sm text-muted-foreground">{{ t('components.cronJobModal.every') }}</span>
        <Input
          id="cron-interval-value"
          v-model.number="intervalValue"
          type="number"
          min="1"
          max="30"
          class="sb-mono w-24"
          aria-describedby="cron-interval-help"
        />
        <Select v-model="intervalUnit">
          <SelectTrigger class="w-28 shrink-0" :aria-label="t('components.cronJobModal.intervalUnitAria')"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="minute">{{ t('components.cronJobModal.unitMinute') }}</SelectItem>
            <SelectItem value="hour">{{ t('components.cronJobModal.unitHour') }}</SelectItem>
            <SelectItem value="day">{{ t('components.cronJobModal.unitDay') }}</SelectItem>
          </SelectContent>
        </Select>
        <span class="text-sm text-muted-foreground">{{ t('components.cronJobModal.runOnce') }}</span>
      </div>
      <FieldDescription id="cron-interval-help">{{ t('components.cronJobModal.intervalHint') }}</FieldDescription>
    </Field>

    <Field v-else class="pt-3">
      <FieldLabel for="cron-run-at">{{ t('components.cronJobModal.runAtLabel') }}</FieldLabel>
      <Input
        id="cron-run-at"
        v-model="runAtLocal"
        type="datetime-local"
        class="sb-mono w-full sm:w-64"
        aria-describedby="cron-run-at-help"
      />
      <FieldDescription id="cron-run-at-help">{{ t('components.cronJobModal.runAtHint') }}</FieldDescription>
    </Field>
  </FieldSet>
</template>

<script setup lang="ts">
/**
 * 定时任务的「时间执行」字段组（从 CronJobModal 拆出）。
 *
 * 三个模式（cron / interval / once）的字段互斥渲染，模板里已含
 * aria-describedby 与 label 关联 —— 这些是无障碍契约，拆出时原样保留
 * （tests/CronJobModal.test.ts 逐条断言）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { CRON_PRESETS, type IntervalUnit, type ScheduleKind } from './useCronJobForm'

const scheduleKind = defineModel<ScheduleKind>('scheduleKind', { required: true })
const cronExpr = defineModel<string>('cronExpr', { required: true })
const runAtLocal = defineModel<string>('runAtLocal', { required: true })
const intervalValue = defineModel<number>('intervalValue', { required: true })
const intervalUnit = defineModel<IntervalUnit>('intervalUnit', { required: true })
const preset = defineModel<string | undefined>('preset')

const { t } = useI18n()

const presets = CRON_PRESETS

/** Select 不接受 undefined 作为 model value，这里归一成空串。 */
const presetValue = computed({
  get: () => preset.value ?? '',
  set: (v: string) => {
    preset.value = v || undefined
  }
})
</script>
