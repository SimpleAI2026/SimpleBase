<template>
  <div class="flex flex-col gap-4">
    <div v-if="title" class="flex flex-wrap items-baseline gap-3">
      <h2 class="m-0 font-serif text-xl font-semibold tracking-tight text-foreground">{{ title }}</h2>
      <span v-if="subtitle" class="text-sm text-muted-foreground">{{ subtitle }}</span>
    </div>
    <slot />
  </div>
</template>
<script setup lang="ts">
defineProps<{ title?: string; subtitle?: string }>()
</script>
