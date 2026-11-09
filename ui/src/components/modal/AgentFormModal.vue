<template>
  <SbModal
    :open="open"
    :title="editing ? t('pages.agents.editTitle') : t('pages.agents.createTitle')"
    :confirm-loading="saving"
    @ok="emit('save')"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <FieldGroup>
      <Field>
        <FieldLabel for="agent-name">{{ t('pages.agents.formName') }}</FieldLabel>
        <Input id="agent-name" v-model="form.name" placeholder="Database" />
      </Field>
      <Field>
        <FieldLabel>{{ t('pages.agents.formModule') }}</FieldLabel>
        <Select :model-value="form.module" @update:model-value="(v: string) => emit('module-change', v)">
          <SelectTrigger class="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem
                v-for="opt in moduleOptions"
                :key="opt.value"
                :value="opt.value"
                :disabled="opt.disabled"
              >
                {{ opt.label }}{{ opt.disabled ? t('pages.agents.moduleUnconfigured') : '' }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <Field>
        <FieldLabel for="agent-model">{{ t('pages.agents.formModel') }}</FieldLabel>
        <!--
          model_override 是自由文本：Combobox 的选中值与输入框搜索词绑定到同一个字段，
          这样手输的模型名在失焦重置搜索词时不会被清掉，候选项只是建议。
        -->
        <Combobox
          :model-value="form.model_override"
          open-on-click
          @update:model-value="(v: string) => (form.model_override = v ?? '')"
        >
          <ComboboxAnchor>
            <ComboboxInput
              id="agent-model"
              v-model="form.model_override"
              :placeholder="defaultModel ? t('pages.agents.modelPlaceholderDefault', { model: defaultModel }) : t('pages.agents.modelPlaceholder')"
            />
          </ComboboxAnchor>
          <ComboboxList>
            <ComboboxViewport>
              <ComboboxEmpty>{{ t('pages.agents.modelNoSuggestion') }}</ComboboxEmpty>
              <ComboboxGroup>
                <ComboboxItem v-for="model in modelOptions" :key="model" :value="model">{{ model }}</ComboboxItem>
              </ComboboxGroup>
            </ComboboxViewport>
          </ComboboxList>
        </Combobox>
      </Field>
      <Field>
        <FieldLabel for="agent-desc">{{ t('common.description') }}</FieldLabel>
        <Input id="agent-desc" v-model="form.description" />
      </Field>
      <Field>
        <FieldLabel for="agent-prompt">System prompt</FieldLabel>
        <Textarea id="agent-prompt" v-model="form.system_prompt" :rows="4" :placeholder="t('pages.agents.formPromptPlaceholder')" />
      </Field>
      <Field>
        <FieldLabel>{{ t('pages.agents.formTools') }}</FieldLabel>
        <div class="flex flex-wrap gap-2">
          <Badge
            v-for="id in toolOptions"
            :key="id.value"
            as="button"
            type="button"
            :aria-pressed="form.tool_ids.includes(id.value)"
            :title="t('pages.agents.toolLabel', { label: id.label })"
            :variant="form.tool_ids.includes(id.value) ? 'default' : 'outline'"
            class="h-auto min-h-8 cursor-pointer"
            @click="emit('toggle-tool', id.value)"
          >
            {{ id.label }}
          </Badge>
        </div>
      </Field>
      <Field>
        <FieldLabel>{{ t('pages.agents.formWritable') }}</FieldLabel>
        <div class="flex items-center gap-2">
          <Switch
            :model-value="form.writable"
            data-agent-writable
            :aria-label="t('pages.agents.formWritable')"
            @update:model-value="(v: boolean) => emit('update:writable', v)"
          />
          <span class="text-xs text-muted-foreground">{{ t('pages.agents.formWritableHint') }}</span>
        </div>
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * Agent 新建/编辑弹窗（从 pages/AgentManager.vue 拆出）。
 *
 * 表单状态由宿主持有（`form` 以 v-model 传入），本组件只做布局与事件转发，
 * 这样宿主的 `saveAgent` / `openEdit` 逻辑保持不变、测试表面也不变。
 */
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxViewport,
} from '@/components/ui/combobox'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import SbModal from '../modal/SbModal.vue'

export interface AgentFormState {
  name: string
  module: string
  description: string
  system_prompt: string
  model_override: string
  tool_ids: string[]
  /** 是否允许写（planv6.0 v2 §3.2）：默认关闭。 */
  writable: boolean
}

export interface AgentModuleOption {
  label: string
  value: string
  disabled: boolean
}

export interface AgentToolOption {
  label: string
  value: string
}

const props = defineProps<{
  open: boolean
  /** 编辑目标；null = 新建（只用于决定标题与是否回填模块默认值） */
  editing: boolean
  saving: boolean
  form: AgentFormState
  moduleOptions: AgentModuleOption[]
  toolOptions: AgentToolOption[]
  modelOptions: string[]
  defaultModel: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  save: []
  'module-change': [value: string]
  'toggle-tool': [id: string]
  'update:writable': [value: boolean]
}>()

const { t } = useI18n()

void props
</script>
