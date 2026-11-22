<template>
  <!-- AgentGroupMembersModal：群成员弹窗（planv6.7 v2 §4.3）。
       两个页签：加入已有助手（多选）/ 新建助手并加入。
       提交走宿主；409 版本冲突由宿主置 conflicted，用户重新加载后丢弃本地修改。 -->
  <SbModal
    :open="open"
    :title="t('components.agentGroupMembers.title', { name: groupName })"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !submittable }"
    @ok="submit"
    @update:open="(v: boolean) => emit('update:open', v)"
  >
    <div v-if="conflicted" class="mb-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive" role="alert" data-group-conflict>
      {{ t('components.agentTagModal.conflict') }}
      <Button size="xs" variant="outline" class="ml-2" @click="emit('reload')">{{ t('components.agentTagModal.reload') }}</Button>
    </div>

    <!-- 页签：加入已有 / 新建并加入。 -->
    <div class="mb-3 flex gap-1" role="tablist">
      <button
        v-for="tab in TABS"
        :key="tab"
        type="button"
        role="tab"
        :aria-selected="mode === tab"
        :data-group-tab="tab"
        class="rounded-md px-2.5 py-1 text-xs"
        :class="mode === tab ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground'"
        @click="mode = tab"
      >{{ t(`components.agentGroupMembers.tab.${tab}`) }}</button>
    </div>

    <!-- 加入已有助手：多选，已在本群的置为不可选。 -->
    <div v-if="mode === 'existing'" class="flex max-h-72 flex-col gap-1 overflow-y-auto rounded-md border border-border p-2" data-group-existing-list>
      <label
        v-for="a in agents"
        :key="a.id"
        class="flex min-h-8 items-center gap-2 text-xs"
        :class="isMember(a.id) ? 'opacity-50' : 'cursor-pointer'"
      >
        <input
          type="checkbox"
          class="size-3.5 accent-[var(--primary)]"
          :checked="selected.includes(a.id)"
          :disabled="isMember(a.id)"
          :aria-label="a.name"
          :data-group-agent-check="a.id"
          @change="toggle(a.id)"
        />
        <component :is="iconForAgentModule(a.module)" class="size-3.5 shrink-0 opacity-80" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">{{ a.name }}</span>
        <span class="shrink-0 text-muted-foreground">{{ isMember(a.id) ? t('components.agentGroupMembers.joined') : a.module }}</span>
      </label>
      <p v-if="!agents.length" class="px-1 py-2 text-xs text-muted-foreground">{{ t('pages.agents.emptyListDesc') }}</p>
    </div>

    <!-- 新建助手并加入。 -->
    <FieldGroup v-else>
      <Field>
        <FieldLabel for="group-new-agent-name">{{ t('components.agentGroupMembers.agentName') }}</FieldLabel>
        <Input id="group-new-agent-name" v-model="newName" :placeholder="t('components.agentGroupMembers.agentNamePlaceholder')" data-group-new-name />
      </Field>
      <Field>
        <FieldLabel>{{ t('components.agentGroupMembers.agentModule') }}</FieldLabel>
        <div class="flex flex-wrap gap-1">
          <button
            v-for="m in modules"
            :key="m.id"
            type="button"
            class="rounded-full border px-2.5 py-1 text-xs"
            :class="newModule === m.id ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:text-foreground'"
            :disabled="m.id === 'sandbox' && m.sandbox_available !== true"
            :data-group-new-module="m.id"
            @click="pickModule(m.id)"
          >{{ m.name }}</button>
        </div>
      </Field>
      <Field>
        <FieldLabel for="group-new-agent-desc">{{ t('components.agentGroupMembers.agentDesc') }}</FieldLabel>
        <Input id="group-new-agent-desc" v-model="newDescription" :placeholder="t('components.agentGroupMembers.agentDescPlaceholder')" data-group-new-desc />
      </Field>
    </FieldGroup>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 群成员弹窗：加入已有助手 或 新建助手并加入（planv6.7 v2 §4.3）。
 *
 * 只负责收集输入并 emit；版本冲突、写库、补偿都由宿主处理。
 * 「新建并加入」用一个请求提交（服务端组合事务），避免先建助手再入群的两步失败。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import SbModal from '@/components/modal/SbModal.vue'
import { iconForAgentModule } from '@/components/nav-icons'
import type { AgentGroupMembersInput, AgentModuleInfo, MentionAgentLike } from '@/services/types'

type Mode = 'existing' | 'create'

const props = defineProps<{
  open: boolean
  /** 群显示名（预置群已本地化）。 */
  groupName: string
  /** 项目内全部助手（成员会被置灰）。 */
  agents: MentionAgentLike[]
  /** 已是成员的助手 ID。 */
  memberIds: string[]
  /** 可选模块（沙盒未启用时禁用）。 */
  modules: AgentModuleInfo[]
  /** 默认页签（从「新建助手并加入」入口打开时传 create）。 */
  initialMode?: Mode
  saving?: boolean
  conflicted?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:open', v: boolean): void
  (e: 'save', input: AgentGroupMembersInput): void
  (e: 'reload'): void
}>()

const { t } = useI18n()

const TABS: Mode[] = ['existing', 'create']

const mode = ref<Mode>('existing')
const selected = ref<string[]>([])
const newName = ref('')
const newModule = ref('database')
const newDescription = ref('')

watch(
  () => [props.open, props.initialMode] as const,
  ([open, initial]) => {
    if (!open) return
    mode.value = initial ?? 'existing'
    selected.value = []
    newName.value = ''
    newDescription.value = ''
    // 模块默认取第一个可用项（沙盒未启用时不选中）。
    const first = props.modules.find((m) => !(m.id === 'sandbox' && m.sandbox_available !== true))
    newModule.value = first?.id || 'database'
  },
  { immediate: true }
)

/** 是否已是成员：已在本群的助手不能再选。 */
function isMember(id: string): boolean {
  return props.memberIds.includes(id)
}

function toggle(id: string) {
  if (isMember(id)) return
  selected.value = selected.value.includes(id)
    ? selected.value.filter((x) => x !== id)
    : [...selected.value, id]
}

function pickModule(id: string) {
  newModule.value = id
  // 名称留空时按模块预填，减少一次输入。
  if (!newName.value.trim()) {
    const m = props.modules.find((item) => item.id === id)
    if (m) newName.value = m.name
  }
}

const submittable = computed(() => {
  if (mode.value === 'existing') return selected.value.length > 0
  return newName.value.trim().length > 0 && newModule.value.length > 0
})

function submit() {
  if (!submittable.value) return
  if (mode.value === 'existing') {
    emit('save', { agentIds: [...selected.value] })
    return
  }
  emit('save', {
    newAgent: {
      name: newName.value.trim(),
      module: newModule.value,
      description: newDescription.value.trim()
    }
  })
}
</script>
