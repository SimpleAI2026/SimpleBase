<template>
  <SbModal
    :open="open"
    :title="isEdit ? t('components.cronJobModal.titleEdit') : t('components.cronJobModal.titleCreate')"
    :max-width="720"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !canSave }"
    @update:open="$emit('update:open', $event)"
    @ok="save"
  >
    <div class="flex max-h-[70vh] flex-col gap-4 overflow-y-auto px-2 py-3">
      <FieldGroup>
        <Field :data-invalid="nameError ? true : undefined">
          <FieldLabel for="cron-name">{{ t('components.cronJobModal.name') }}</FieldLabel>
          <Input
            id="cron-name"
            v-model="form.name"
            class="sb-mono"
            :disabled="isEdit"
            placeholder="nightly-refresh"
            :aria-invalid="nameError ? true : undefined"
            :aria-describedby="nameError ? 'cron-name-help cron-name-error' : 'cron-name-help'"
          />
          <FieldDescription id="cron-name-help">{{ isEdit ? t('components.cronJobModal.nameImmutable') : t('components.cronJobModal.nameHint') }}</FieldDescription>
          <FieldError v-if="nameError" id="cron-name-error">{{ nameError }}</FieldError>
        </Field>
        <Field>
          <FieldLabel for="cron-description">{{ t('components.cronJobModal.description') }}</FieldLabel>
          <Input id="cron-description" v-model="form.description" :placeholder="t('components.cronJobModal.descriptionPlaceholder')" />
        </Field>
      </FieldGroup>

      <Separator />

      <CronJobScheduleFields
        v-model:schedule-kind="form.scheduleKind"
        v-model:cron-expr="form.cronExpr"
        v-model:run-at-local="form.runAtLocal"
        v-model:interval-value="intervalValue"
        v-model:interval-unit="intervalUnit"
        v-model:preset="preset"
      />

      <Separator />

      <FieldSet>
        <FieldLegend>{{ t('components.cronJobModal.targetKind') }}</FieldLegend>
        <FieldContent>
          <div class="flex flex-wrap items-center gap-6 pt-1.5">
            <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
              <input
                v-model="form.targetKind"
                type="radio"
                name="target-kind"
                value="function"
                class="size-4 shrink-0 accent-primary"
              />
              <span>{{ t('components.cronJobModal.targetFunction') }}</span>
            </label>
            <label class="flex cursor-pointer items-center gap-2 text-sm font-normal">
              <input
                v-model="form.targetKind"
                type="radio"
                name="target-kind"
                value="agent"
                class="size-4 shrink-0 accent-primary"
              />
              <span>{{ t('components.cronJobModal.targetAgent') }}</span>
            </label>
          </div>
        </FieldContent>
      </FieldSet>

      <FieldGroup v-if="form.targetKind === 'agent'">
        <Field>
          <FieldLabel id="cron-agent-label">{{ t('components.cronJobModal.selectAgent') }}</FieldLabel>
          <SbBlockSkeleton v-if="agentsPending && agents.length === 0" variant="lines" :count="2" />
          <Select v-else v-model="form.agentId">
            <SelectTrigger aria-labelledby="cron-agent-label" :disabled="agentsFailed">
              <SelectValue :placeholder="t('components.cronJobModal.selectAgent')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="a in agents" :key="a.id" :value="a.id">{{ a.name }}</SelectItem>
            </SelectContent>
          </Select>
          <Alert v-if="agentsFailed" variant="destructive">
            <AlertTitle>{{ t('error.loadFailed') }}</AlertTitle>
            <AlertDescription>{{ t('components.cronJobModal.loadAgentsFailed') }}</AlertDescription>
          </Alert>
          <Button v-if="agentsFailed" variant="outline" size="sm" class="w-fit" @click="loadAgents">{{ t('common.retry') }}</Button>
          <FieldDescription v-if="!agentsPending && !agentsFailed && agents.length === 0">
            {{ t('components.cronJobModal.noAgents') }}
          </FieldDescription>
        </Field>
        <Field :data-invalid="agentPromptError ? true : undefined">
          <FieldLabel for="cron-agent-prompt">{{ t('components.cronJobModal.agentPrompt') }}</FieldLabel>
          <Textarea
            id="cron-agent-prompt"
            v-model="form.agentPrompt"
            rows="6"
            :aria-invalid="agentPromptError ? true : undefined"
            :aria-describedby="agentPromptError ? 'cron-prompt-help cron-prompt-error' : 'cron-prompt-help'"
          />
          <FieldDescription id="cron-prompt-help">
            {{ t('components.cronJobModal.agentPromptHint') }}
            <span class="sb-mono">{{ t('components.cronJobModal.agentPromptCount', { used: agentPromptBytes, max: CRON_AGENT_PROMPT_MAX_BYTES }) }}</span>
          </FieldDescription>
          <FieldError v-if="agentPromptError" id="cron-prompt-error">{{ agentPromptError }}</FieldError>
        </Field>
      </FieldGroup>

      <FieldGroup v-else>
        <Field>
          <FieldLabel id="cron-file-label">{{ t('components.cronJobModal.funcFile') }}</FieldLabel>
          <SbBlockSkeleton v-if="pending && gofunctions.length === 0" variant="lines" :count="3" />
          <Select v-else v-model="form.funcFile" @update:model-value="form.funcExport = ''">
            <SelectTrigger aria-labelledby="cron-file-label" :disabled="showError">
              <SelectValue :placeholder="t('components.cronJobModal.selectFile')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="g in availableFunctions" :key="g.id" :value="g.name">
                {{ g.file }}
              </SelectItem>
            </SelectContent>
          </Select>
          <Alert v-if="showError" variant="destructive">
            <AlertTitle>{{ t('error.loadFailed') }}</AlertTitle>
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
          <Button v-if="showError" variant="outline" size="sm" class="w-fit" @click="loadFunctions">{{ t('common.retry') }}</Button>
          <FieldDescription v-if="settled && !pending && !showError && gofunctions.length === 0">
            {{ t('components.cronJobModal.noFunctions') }}
            <RouterLink :to="{ name: 'gofunctions' }" class="text-primary underline">{{ t('components.cronJobModal.noFunctionsLink') }}</RouterLink>
          </FieldDescription>
          <FieldDescription v-else-if="gofunctions.length > 0 && !availableFunctions.length">
            {{ t('components.cronJobModal.noPublished') }}
            <RouterLink :to="{ name: 'gofunctions' }" class="text-primary underline">{{ t('components.cronJobModal.noPublishedLink') }}</RouterLink>
          </FieldDescription>
          <FieldDescription v-else-if="unpublishedCount > 0">
            {{ t('components.cronJobModal.unpublishedHint', { n: unpublishedCount }) }}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel id="cron-export-label">{{ t('components.cronJobModal.funcExport') }}</FieldLabel>
          <Select v-model="form.funcExport">
            <SelectTrigger aria-labelledby="cron-export-label">
              <SelectValue :placeholder="t('components.cronJobModal.selectFunction')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="fn in currentExports" :key="fn" :value="fn">{{ fn }}</SelectItem>
            </SelectContent>
          </Select>
          <FieldDescription>{{ t('components.cronJobModal.funcExportHint') }}</FieldDescription>
        </Field>
      </FieldGroup>

      <Field v-if="form.targetKind === 'function'" :data-invalid="inputJsonError ? true : undefined">
        <FieldLabel for="cron-input-json">{{ t('components.cronJobModal.inputJson') }}</FieldLabel>
        <Textarea
          id="cron-input-json"
          v-model="form.inputJson"
          rows="3"
          class="sb-mono"
          :aria-invalid="inputJsonError ? true : undefined"
          :aria-describedby="inputJsonError ? 'cron-json-help cron-json-error' : 'cron-json-help'"
          placeholder='{ "name": "cron" }'
        />
        <FieldDescription id="cron-json-help">{{ t('components.cronJobModal.inputJsonHint') }}</FieldDescription>
        <FieldError v-if="inputJsonError" id="cron-json-error">{{ inputJsonError }}</FieldError>
      </Field>

      <Alert v-if="form.targetKind === 'function'">
        <InfoIcon class="size-4" />
        <AlertDescription>
          {{ t('components.cronJobModal.inputJsonTip') }}
        </AlertDescription>
      </Alert>
    </div>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 定时任务新建/编辑 Modal（ui-cronjob-plan §7.3）。
 *
 * 表单状态与校验在 `cronjob/useCronJobForm`，调度字段在
 * `cronjob/CronJobScheduleFields`；本文件只做布局编排。
 * 为兼容既有测试，composable 的字段在这里原样暴露（`vm.form` 等）。
 */
import { computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { InfoIcon } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
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
import CronJobScheduleFields from '../cronjob/CronJobScheduleFields.vue'
import { useCronJobForm } from '../cronjob/useCronJobForm'
import { CRON_AGENT_PROMPT_MAX_BYTES, type CronJobItem } from '../../services/api'
import { useProjectStore } from '../../stores/project'

const props = defineProps<{
  open: boolean
  /** 编辑目标；undefined = 新建 */
  target?: CronJobItem
  /** 新建时预选的助手 id（AgentManager 跳转而来） */
  presetAgentId?: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const { t } = useI18n()
const projectStore = useProjectStore()

const {
  form,
  isEdit,
  intervalValue,
  intervalUnit,
  intervalSeconds,
  preset,
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
} = useCronJobForm({
  target: toRef(props, 'target'),
  projectId: computed(() => projectStore.projectId),
  open: toRef(props, 'open'),
  presetAgentId: toRef(props, 'presetAgentId'),
  onSaved: () => {
    emit('saved')
    emit('update:open', false)
  }
})
</script>
