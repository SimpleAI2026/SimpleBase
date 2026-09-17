<template>
  <Tabs :model-value="modelValue" class="gap-0" @update:model-value="$emit('update:modelValue', String($event))">
    <TabsList variant="line" class="w-full justify-start rounded-none bg-card px-5 pt-3">
      <TabsTrigger v-for="m in modules" :key="m.id" :value="m.id">
        {{ m.title }}
      </TabsTrigger>
    </TabsList>
  </Tabs>
</template>

<script setup lang="ts">
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { DocModule } from '../../docs/catalog'

defineProps<{
  modules: DocModule[]
  modelValue: string
}>()

defineEmits<{
  'update:modelValue': [id: string]
}>()
</script>
