<template>
  <Button variant="outline" size="sm" :disabled="pending || disabled" @click="emit('click')">
    <Spinner v-if="pending" data-icon="inline-start" />
    <RefreshCwIcon v-else data-icon="inline-start" />
    {{ label || t('common.refresh') }}
  </Button>
</template>

<script setup lang="ts">
/**
 * 列表页 / 面板通用的「刷新」按钮（planv6.7 ui-simplify-ux-plan S2）。
 * 固定 outline + sm，加载中显示 Spinner 并禁用；文案缺省 common.refresh。
 */
import { useI18n } from 'vue-i18n'
import { RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

defineProps<{
  pending?: boolean
  disabled?: boolean
  label?: string
}>()

const emit = defineEmits<{ click: [] }>()

const { t } = useI18n()
</script>
