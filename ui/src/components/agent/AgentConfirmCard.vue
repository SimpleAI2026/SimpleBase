<template>
  <!-- AgentConfirmCard：对话内的行内确认卡（planv6.0 §3.6 #1）。
       取代原来的全屏 AlertDialog：不遮挡上下文，用户能同时看到工具调用与回答。
       出现时自动聚焦「确认执行」，Esc 等于取消；不用 Enter 确认，避免与输入框的 Enter 发送冲突。 -->
  <Card
    size="sm"
    data-slot="agent-confirm-card"
    class="border-warning/50 bg-warning/5"
    @keydown.esc.stop.prevent="$emit('deny')"
  >
    <CardHeader>
      <CardTitle class="flex items-center gap-1.5 text-xs">
        <ShieldAlertIcon class="size-3.5 shrink-0 text-warning" aria-hidden="true" />
        <span>{{ t('components.agentConfirm.title') }}</span>
      </CardTitle>
    </CardHeader>
    <CardContent class="space-y-2 px-3 text-xs">
      <p class="text-muted-foreground">{{ message }}</p>
      <pre
        v-if="argv.length"
        class="max-h-32 overflow-auto rounded-md bg-muted px-2 py-1.5 whitespace-pre-wrap text-foreground"
      >{{ argv.join(' ') }}</pre>
      <div class="flex justify-end gap-2">
        <Button size="sm" variant="outline" @click="$emit('deny')">
          {{ t('common.cancel') }}
        </Button>
        <Button ref="confirmBtn" size="sm" @click="$emit('confirm')">
          {{ t('components.agentConfirm.confirm') }}
        </Button>
      </div>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldAlertIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

defineProps<{
  /** 后端给出的确认说明。 */
  message: string
  /** 将被执行的 argv，空数组时不展示代码块。 */
  argv: string[]
}>()

defineEmits<{
  (e: 'confirm'): void
  (e: 'deny'): void
}>()

const { t } = useI18n()

const confirmBtn = ref<{ $el?: HTMLElement } | null>(null)

/** 出现即聚焦「确认执行」，键盘用户可直接操作，Esc 由根节点处理。 */
onMounted(() => {
  confirmBtn.value?.$el?.focus?.()
})
</script>
