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
<!-- 样式收敛到 utilities.css 的 .sb-page / .sb-page-title，组件不再重复定义 -->
