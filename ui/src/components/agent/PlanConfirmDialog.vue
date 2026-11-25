<template>
  <SbModal
    :open="open"
    :title="t('components.agentWorkspace.planConfirmTitle')"
    :description="t('components.agentWorkspace.planConfirmDescription')"
    :max-width="720"
    @update:open="() => undefined"
  >
    <div v-if="plan" class="flex flex-col gap-4" data-plan-confirm>
      <section>
        <h3 class="text-xs font-medium text-muted-foreground">{{ t('components.agentWorkspace.planGoal') }}</h3>
        <p class="mt-1 whitespace-pre-wrap text-sm">{{ plan.goal }}</p>
        <ul v-if="plan.confirm.reasons.length" class="mt-2 list-disc pl-5 text-xs text-amber-600">
          <li v-for="reason in plan.confirm.reasons" :key="reason">{{ reason }}</li>
        </ul>
      </section>
      <section v-if="plan.steps.length">
        <h3 class="mb-1 text-xs font-medium text-muted-foreground">{{ t('components.agentWorkspace.planSteps') }}</h3>
        <ol class="flex flex-col gap-1 text-xs">
          <li v-for="step in plan.steps" :key="step.id" class="rounded bg-muted/50 px-2 py-1.5">
            <span class="font-medium">{{ step.title }}</span>
            <span v-if="step.note" class="ml-2 text-muted-foreground">{{ step.note }}</span>
          </li>
        </ol>
      </section>
      <section>
        <h3 class="mb-1 text-xs font-medium text-muted-foreground">{{ t('components.agentWorkspace.planTasks') }}</h3>
        <div class="flex flex-col gap-2">
          <div
            v-for="dispatch in plan.dispatches"
            :key="dispatch.ref"
            class="rounded-lg border border-border p-2"
            :data-plan-ref="dispatch.ref"
          >
            <div class="mb-1 flex items-center justify-between gap-2 text-xs">
              <span class="font-medium">{{ dispatch.ref }} · {{ names[dispatch.targetId] || dispatch.targetId }}</span>
              <Button
                type="button"
                size="xs"
                variant="outline"
                :disabled="busy"
                :data-plan-drop="dispatch.ref"
                @click="dropped[dispatch.ref] = !dropped[dispatch.ref]"
              >
                {{ dropped[dispatch.ref] ? t('components.agentWorkspace.planRestore') : t('components.agentWorkspace.planDrop') }}
              </Button>
            </div>
            <Textarea
              v-model="messages[dispatch.ref]"
              :disabled="busy || dropped[dispatch.ref]"
              :aria-label="t('components.agentWorkspace.planEditMessage', { ref: dispatch.ref })"
              :data-plan-message="dispatch.ref"
              rows="2"
            />
            <p v-if="dispatch.dependsOn.length" class="mt-1 text-[10px] text-muted-foreground">
              {{ dispatch.dependsOn.join(' → ') }} → {{ dispatch.ref }}
            </p>
          </div>
        </div>
      </section>
    </div>
    <template #footer>
      <Button variant="outline" :disabled="busy" data-plan-reject @click="emit('submit', false, [])">
        {{ t('components.agentWorkspace.planReject') }}
      </Button>
      <Button :disabled="busy" data-plan-approve @click="approve">
        {{ t('components.agentWorkspace.planApprove') }}
      </Button>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import SbModal from '@/components/modal/SbModal.vue'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import type { AgentPlanDecision, AgentPlanEdit } from '@/services/types'

interface Props {
  open: boolean
  plan: AgentPlanDecision | null
  names?: Record<string, string>
  busy?: boolean
}

const props = withDefaults(defineProps<Props>(), { names: () => ({}), busy: false })
const emit = defineEmits<{
  submit: [approve: boolean, edits: AgentPlanEdit[]]
}>()
const { t } = useI18n()
const messages = reactive<Record<string, string>>({})
const dropped = reactive<Record<string, boolean>>({})

watch(() => props.plan, (plan) => {
  for (const key of Object.keys(messages)) delete messages[key]
  for (const key of Object.keys(dropped)) delete dropped[key]
  for (const dispatch of plan?.dispatches ?? []) {
    messages[dispatch.ref] = dispatch.message
    dropped[dispatch.ref] = false
  }
}, { immediate: true, deep: true })

function approve(): void {
  const edits: AgentPlanEdit[] = []
  for (const dispatch of props.plan?.dispatches ?? []) {
    if (dropped[dispatch.ref]) edits.push({ ref: dispatch.ref, drop: true })
    else if (messages[dispatch.ref] !== dispatch.message) edits.push({ ref: dispatch.ref, message: messages[dispatch.ref] })
  }
  emit('submit', true, edits)
}
</script>
