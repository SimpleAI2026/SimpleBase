<template>
  <a-layout class="sb-layout">
    <a-layout-sider
      class="sb-sider"
      collapsible
      v-model:collapsed="collapsed"
      :width="232"
      :collapsed-width="64"
    >
      <div class="sb-brand" :class="{ 'is-collapsed': collapsed }">
        <div class="sb-brand-logo">S</div>
        <transition name="sb-fade">
          <span v-if="!collapsed" class="sb-brand-text">SimpleBase</span>
        </transition>
      </div>
      <NavMenu />
    </a-layout-sider>

    <a-layout class="sb-main">
      <a-layout-header class="sb-header">
        <div class="sb-header-left">
          <a-breadcrumb>
            <a-breadcrumb-item>Admin</a-breadcrumb-item>
            <a-breadcrumb-item>{{ currentTitle }}</a-breadcrumb-item>
          </a-breadcrumb>
        </div>
        <div class="sb-header-right">
          <a-tooltip title="GitHub">
            <a href="https://github.com/voocel/SimpleBase" target="_blank" class="sb-icon-link">
              <GithubOutlined />
            </a>
          </a-tooltip>
          <a-tooltip title="刷新页面">
            <ReloadOutlined class="sb-icon-link" @click="reload" />
          </a-tooltip>
          <a-avatar style="background-color: var(--sb-primary)" size="small">A</a-avatar>
        </div>
      </a-layout-header>

      <a-layout-content class="sb-content">
        <router-view v-slot="{ Component }">
          <transition name="sb-fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>
<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { GithubOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import NavMenu from '../components/NavMenu.vue'

const collapsed = ref(false)
const route = useRoute()

const titleMap: Record<string, string> = {
  dashboard: '监控大盘',
  data: '数据管理',
  s3: 'S3 对象存储',
  project: '项目管理',
  faas: 'FaaS 函数',
  logs: '日志管理'
}
const currentTitle = computed(() => titleMap[route.name as string] || 'SimpleBase')

function reload() {
  location.reload()
}
</script>
<style scoped>
.sb-layout {
  min-height: 100vh;
}
.sb-sider {
  background: #161b2e !important;
  box-shadow: 2px 0 12px rgba(15, 23, 42, 0.1);
  position: relative;
  z-index: 10;
}
.sb-sider :deep(.ant-layout-sider-children) {
  display: flex;
  flex-direction: column;
}
.sb-brand {
  height: var(--sb-header-height);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  overflow: hidden;
}
.sb-brand-logo {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--sb-primary), var(--sb-accent));
  color: #fff;
  font-weight: 800;
  font-size: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.sb-brand-text {
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.02em;
  white-space: nowrap;
}
.sb-brand.is-collapsed {
  justify-content: center;
  padding: 0;
}
.sb-main {
  background: var(--sb-bg);
}
.sb-header {
  background: var(--sb-surface) !important;
  height: var(--sb-header-height) !important;
  line-height: var(--sb-header-height) !important;
  padding: 0 20px !important;
  border-bottom: 1px solid var(--sb-border-soft);
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.03);
}
.sb-header-left {
  display: flex;
  align-items: center;
}
.sb-header-right {
  display: flex;
  align-items: center;
  gap: 14px;
}
.sb-icon-link {
  color: var(--sb-text-secondary);
  font-size: 16px;
  cursor: pointer;
  transition: var(--sb-transition);
}
.sb-icon-link:hover {
  color: var(--sb-primary);
}
.sb-content {
  margin: 18px;
  padding: 0;
}
</style>
