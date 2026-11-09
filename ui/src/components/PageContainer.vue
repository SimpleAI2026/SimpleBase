<template>
  <div class="flex min-w-0 flex-col gap-4 sm:gap-5">
    <p v-if="subtitle" class="m-0 text-sm text-muted-foreground">{{ subtitle }}</p>
    <slot />
  </div>
</template>
<script setup lang="ts">
defineProps<{ subtitle?: string }>()
</script>
