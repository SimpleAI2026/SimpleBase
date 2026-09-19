<template>
  <Tabs :model-value="modelValue" class="gap-0" @update:model-value="$emit('update:modelValue', String($event))">
    <div class="mx-auto w-full max-w-[1200px] px-4 md:px-6">
      <TabsList variant="line" class="h-11 w-full justify-start gap-2 rounded-none bg-transparent">
        <TabsTrigger v-for="m in modules" :key="m.id" :value="m.id" class="px-2.5 text-[13.5px]">
          {{ m.title }}
        </TabsTrigger>
      </TabsList>
    </div>
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
