<template>
  <div class="sb-empty-state">
    <a-empty :description="description" :image="simpleImage">
      <a-button v-if="actionText" type="primary" @click="$emit('action')">
        <template #icon><PlusOutlined /></template>
        {{ actionText }}
      </a-button>
    </a-empty>
  </div>
</template>
<script setup lang="ts">
import { Empty } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'

/** 空态 + 引导 CTA：空数据不是死胡同 */
defineProps<{
  description?: string
  /** 引导操作按钮文案；缺省不显示按钮 */
  actionText?: string
}>()

defineEmits<{ (e: 'action'): void }>()

const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE
</script>
<style scoped>
.sb-empty-state {
  padding: var(--sb-space-5) 0;
}
</style>
