<template>
  <a-menu
    class="sb-menu"
    theme="dark"
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
    <a-menu-item key="project">
      <ProjectOutlined />
      <span>项目管理</span>
    </a-menu-item>
    <a-menu-item key="faas">
      <CodeOutlined />
      <span>FaaS 函数</span>
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
  CodeOutlined
} from '@ant-design/icons-vue'

const route = useRoute()
const router = useRouter()
const selectedKey = computed(() => (route.name as string) || 'dashboard')
function onClick(e: any) {
  router.push({ name: e.key })
}
</script>
<style scoped>
.sb-menu {
  background: transparent !important;
  border-right: 0 !important;
  padding-top: 8px;
}
.sb-menu :deep(.ant-menu-item) {
  margin: 4px 8px !important;
  border-radius: 8px !important;
  height: 40px !important;
  line-height: 40px !important;
}
.sb-menu :deep(.ant-menu-item-selected) {
  background: linear-gradient(135deg, var(--sb-primary), var(--sb-primary-hover)) !important;
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.35);
}
.sb-menu :deep(.ant-menu-item:hover) {
  color: #fff !important;
  background: rgba(255, 255, 255, 0.06) !important;
}
</style>
