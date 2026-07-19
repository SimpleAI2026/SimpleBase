<template>
  <div class="sb-page">
    <div class="sb-page-title" v-if="title">
      <h2>{{ title }}</h2>
      <span class="sb-subtitle" v-if="subtitle">{{ subtitle }}</span>
    </div>
    <slot />
  </div>
</template>
<script setup lang="ts">
defineProps<{ title?: string; subtitle?: string }>()
</script>
<style scoped>
.sb-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.sb-page-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
}
.sb-page-title h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--sb-text);
  letter-spacing: -0.01em;
}
.sb-page-title .sb-subtitle {
  color: var(--sb-text-secondary);
  font-size: 13px;
}
</style>
