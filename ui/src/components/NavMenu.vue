<template>
  <a-menu
    class="sb-menu"
    theme="light"
    mode="inline"
    :selectedKeys="[selectedKey]"
    @click="onClick"
  >
    <a-menu-item key="dashboard">
      <AppstoreOutlined />
      <span>监控大盘</span>
    </a-menu-item>
    <a-menu-item key="data">
      <DatabaseOutlined />
      <span>数据管理</span>
    </a-menu-item>
    <a-menu-item key="s3">
      <CloudUploadOutlined />
      <span>S3 对象存储</span>
    </a-menu-item>
    <a-menu-item key="faas">
      <CodeOutlined />
      <span>云函数</span>
    </a-menu-item>
    <a-menu-item key="llm">
      <RobotOutlined />
      <span>LLM 对话</span>
    </a-menu-item>
    <a-menu-item key="logs">
      <FileTextOutlined />
      <span>日志管理</span>
    </a-menu-item>
  </a-menu>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  AppstoreOutlined,
  DatabaseOutlined,
  CloudUploadOutlined,
  ProjectOutlined,
  FileTextOutlined,
  CodeOutlined,
  RobotOutlined
} from '@ant-design/icons-vue'

const emit = defineEmits<{ (e: 'navigate'): void }>()
const route = useRoute()
const router = useRouter()
const selectedKey = computed(() => (route.name as string) || 'dashboard')
function onClick(e: any) {
  router.push({ name: e.key })
  emit('navigate')
}
</script>
<style scoped>
.sb-menu {
  background: transparent !important;
  border-right: 0 !important;
  padding-top: 8px;
}
.sb-menu :deep(.ant-menu-item) {
  margin: 4px 10px !important;
  border-radius: 10px !important;
  height: 40px !important;
  line-height: 40px !important;
  color: var(--sb-text-secondary) !important;
  transition: var(--sb-transition);
}
.sb-menu :deep(.ant-menu-item-selected) {
  background: var(--sb-primary-light) !important;
  color: var(--sb-primary) !important;
  font-weight: 600;
}
.sb-menu :deep(.ant-menu-item-selected .anticon) {
  color: var(--sb-primary) !important;
}
.sb-menu :deep(.ant-menu-item:hover:not(.ant-menu-item-selected)) {
  color: var(--sb-primary) !important;
  background: rgba(31, 30, 29, 0.05) !important;
}
</style>
