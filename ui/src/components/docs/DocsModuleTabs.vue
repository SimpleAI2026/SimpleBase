<template>
  <div class="docs-modules">
    <button
      v-for="m in modules"
      :key="m.id"
      type="button"
      class="docs-module-tab"
      :class="{ active: m.id === modelValue }"
      @click="$emit('update:modelValue', m.id)"
    >
      {{ m.title }}
    </button>
  </div>
</template>

<script setup lang="ts">
import type { DocModule } from '../../docs/catalog'

defineProps<{
  modules: DocModule[]
  modelValue: string
}>()

defineEmits<{
  'update:modelValue': [id: string]
}>()
</script>

<style scoped>
.docs-modules {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 12px 20px 0;
  border-bottom: 1px solid var(--sb-border, #e8e6e0);
  margin-bottom: 0;
  background: var(--sb-surface, #ffffff);
}
.docs-module-tab {
  border: 1px solid transparent;
  background: transparent;
  color: var(--sb-text-secondary, #6b7280);
  padding: 6px 12px;
  border-radius: 6px 6px 0 0;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
}
.docs-module-tab:hover {
  color: var(--sb-text, #1f2937);
  background: rgba(0, 0, 0, 0.03);
}
.docs-module-tab.active {
  color: var(--sb-primary, #d97757);
  border-color: var(--sb-border, #e8e6e0);
  border-bottom-color: var(--sb-surface, #faf9f5);
  background: var(--sb-surface, #faf9f5);
}
</style>
