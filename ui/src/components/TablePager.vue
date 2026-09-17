<template>
  <div v-if="total > 0" class="flex flex-wrap items-center justify-between gap-3 pt-3">
    <p class="text-xs text-muted-foreground">共 {{ total }} 条</p>
    <div v-if="pageCount > 1" class="flex items-center gap-2">
      <Button variant="outline" size="sm" :disabled="page <= 1" @click="emit('update:page', page - 1)">
        <ChevronLeftIcon data-icon="inline-start" />
        上一页
      </Button>
      <span class="text-xs text-muted-foreground">{{ page }} / {{ pageCount }}</span>
      <Button variant="outline" size="sm" :disabled="page >= pageCount" @click="emit('update:page', page + 1)">
        下一页
        <ChevronRightIcon data-icon="inline-end" />
      </Button>
    </div>
  </div>
</template>
<script setup lang="ts">
import { ChevronLeftIcon, ChevronRightIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'

defineProps<{
  page: number
  pageSize: number
  total: number
  pageCount: number
}>()

const emit = defineEmits<{ 'update:page': [value: number] }>()
</script>
