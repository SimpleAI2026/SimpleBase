<template>
  <div class="relative flex min-h-0 flex-1 flex-col">
    <div
      ref="viewport"
      class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto py-1"
      @scroll="onScroll"
    >
      <slot />
    </div>
    <Button
      v-if="!stick"
      class="absolute bottom-3 left-1/2 -translate-x-1/2"
      size="sm"
      variant="outline"
      @click="scrollToEnd"
    >
      {{ t('components.messageScroller.jumpToLatest') }}
    </Button>
  </div>
</template>
<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  followKey?: string | number
}>()

const { t } = useI18n()

const viewport = ref<HTMLElement | null>(null)
const stick = ref(true)

function onScroll() {
  /* v8 ignore next -- mount 后 viewport 必有；测试可先 unmount */
  const el = viewport.value
  if (!el) return
  stick.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

function scrollToEnd() {
  /* v8 ignore next */
  const el = viewport.value
  if (!el) return
  el.scrollTop = el.scrollHeight
  stick.value = true
}

onMounted(() => { scrollToEnd() })

watch(
  () => props.followKey,
  async () => {
    if (!stick.value) return
    await nextTick()
    scrollToEnd()
  }
)

defineExpose({ scrollToEnd })
</script>
