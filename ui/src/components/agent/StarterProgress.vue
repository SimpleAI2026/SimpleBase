<template>
  <!-- 搭建进度（planv6.6 home-starter-prompts §2.3）：状态来自真实工具调用，不靠模型自报。
       失败时停在原阶段，给出「重试本阶段 / 跳过 / 我来接手」。 -->
  <div class="rounded-lg border border-border bg-card px-3 py-2.5" data-starter-progress>
    <ol class="m-0 flex list-none flex-wrap items-center gap-x-1 gap-y-1.5 p-0 text-xs" :aria-label="t('components.starter.progressAria')">
      <li v-for="(item, i) in items" :key="item.stage" class="flex items-center gap-1" :data-stage="item.stage" :data-status="item.status">
        <span
          class="flex items-center gap-1 rounded-full px-2 py-0.5"
          :class="{
            'bg-success/12 text-success': item.status === 'done',
            'bg-primary/10 text-primary': item.status === 'running',
            'bg-destructive/10 text-destructive': item.status === 'failed',
            'text-muted-foreground': item.status === 'pending' || item.status === 'skipped'
          }"
        >
          <CheckIcon v-if="item.status === 'done'" class="size-3" aria-hidden="true" />
          <CircleAlertIcon v-else-if="item.status === 'failed'" class="size-3" aria-hidden="true" />
          <Spinner v-else-if="item.status === 'running'" class="size-3" />
          <MinusIcon v-else-if="item.status === 'skipped'" class="size-3" aria-hidden="true" />
          <span :class="item.status === 'skipped' && 'line-through'">{{ t(stageLabelKey(item.stage)) }}</span>
        </span>
        <span v-if="i < items.length - 1" class="text-muted-foreground" aria-hidden="true">›</span>
      </li>
    </ol>
    <div v-if="failure" class="mt-2 flex flex-wrap items-center gap-2 border-t border-border pt-2" data-starter-failure>
      <p class="m-0 min-w-0 flex-1 text-xs text-destructive">{{ failure.reason }}</p>
      <Button size="xs" variant="outline" data-action="retry" @click="emit('retry')">{{ t('components.starter.retryStage') }}</Button>
      <Button size="xs" variant="outline" data-action="skip" @click="emit('skip')">{{ t('components.starter.skipStage') }}</Button>
      <Button size="xs" variant="ghost" data-action="takeover" @click="emit('takeover')">{{ t('components.starter.takeover') }}</Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckIcon, CircleAlertIcon, MinusIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { stageLabelKey, type StarterStage } from '@/constants/starter-prompts'
import type { StageStatus } from '@/composables/useStarterRun'

export interface StarterProgressItem {
  stage: StarterStage
  status: StageStatus
}

const props = defineProps<{
  items: StarterProgressItem[]
  failure?: { stage: StarterStage; reason: string } | null
}>()

const emit = defineEmits<{ retry: []; skip: []; takeover: [] }>()

const { t } = useI18n()
const items = computed(() => props.items)
</script>
