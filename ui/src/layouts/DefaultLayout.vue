<template>
  <a-layout class="sb-layout">
    <a-layout-sider
      v-if="!isMobile"
      class="sb-sider"
      collapsible
      v-model:collapsed="collapsed"
      :width="siderWidth"
      :collapsed-width="siderCollapsedWidth"
    >
      <div class="sb-brand" :class="{ 'is-collapsed': collapsed }">
        <div class="sb-brand-logo">S</div>
        <transition name="sb-fade">
          <span v-if="!collapsed" class="sb-brand-text">SimpleBase</span>
        </transition>
      </div>
      <NavMenu />
    </a-layout-sider>

    <!-- 移动端抽屉导航 -->
    <a-drawer
      v-if="isMobile"
      v-model:open="drawerOpen"
      placement="left"
      :width="232"
      :closable="false"
      class="sb-drawer"
    >
      <div class="sb-brand">
        <div class="sb-brand-logo">S</div>
        <span class="sb-brand-text">SimpleBase</span>
      </div>
      <NavMenu @navigate="drawerOpen = false" />
    </a-drawer>

    <a-layout class="sb-main">
      <a-layout-header class="sb-header">
        <div class="sb-header-left">
          <MenuOutlined
            v-if="isMobile"
            class="sb-icon-link sb-menu-toggle"
            @click="drawerOpen = true"
          />
          <a-breadcrumb>
            <a-breadcrumb-item>{{ currentTitle }}</a-breadcrumb-item>
          </a-breadcrumb>
        </div>
        <div class="sb-header-right">
          <a-tag v-if="isMock" color="orange" class="sb-mock-tag">Mock 数据</a-tag>
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
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { GithubOutlined, ReloadOutlined, MenuOutlined } from '@ant-design/icons-vue'
import { isMock } from '../services/api'
import NavMenu from '../components/NavMenu.vue'

const collapsed = ref(false)
const drawerOpen = ref(false)
const route = useRoute()
const isMobile = ref(window.innerWidth <= 768)

function onResize() {
  isMobile.value = window.innerWidth <= 768
  if (!isMobile.value) drawerOpen.value = false
}
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => window.removeEventListener('resize', onResize))

const siderWidth = parseInt(
  getComputedStyle(document.documentElement).getPropertyValue('--sb-sider-width') || '232',
  10
)
const siderCollapsedWidth = parseInt(
  getComputedStyle(document.documentElement).getPropertyValue('--sb-sider-collapsed-width') || '64',
  10
)

const titleMap: Record<string, string> = {
  dashboard: '监控大盘',
  data: '数据管理',
  s3: 'S3 对象存储',
  faas: '云函数',
  llm: 'LLM 对话',
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
  background: rgba(250, 249, 245, 0.85) !important;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid var(--sb-border-soft);
  position: relative;
  z-index: 10;
}
.sb-sider :deep(.ant-layout-sider-children) {
  display: flex;
  flex-direction: column;
}
/* 折叠触发器：与磨砂侧边栏融合，去掉 antd 默认的不透明背景 */
.sb-sider :deep(.ant-layout-sider-trigger) {
  background: transparent !important;
  color: var(--sb-text-secondary) !important;
  border-top: 1px solid var(--sb-border-soft);
  transition: var(--sb-transition);
}
.sb-sider :deep(.ant-layout-sider-trigger:hover) {
  color: var(--sb-primary) !important;
  background: rgba(0, 0, 0, 0.04) !important;
}
.sb-brand {
  height: var(--sb-header-height);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 18px;
  border-bottom: 1px solid var(--sb-border-soft);
  overflow: hidden;
}
.sb-brand-logo {
  width: 32px;
  height: 32px;
  border-radius: var(--sb-radius-sm);
  background: var(--sb-primary);
  color: #fff;
  font-family: var(--sb-font-serif);
  font-weight: 600;
  font-size: 17px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.sb-brand-text {
  color: var(--sb-text);
  font-family: var(--sb-font-serif);
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.01em;
  white-space: nowrap;
}
.sb-brand.is-collapsed {
  justify-content: center;
  padding: 0;
}
.sb-main {
  background: transparent;
}
.sb-header {
  background: rgba(250, 249, 245, 0.85) !important;
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  height: var(--sb-header-height) !important;
  line-height: var(--sb-header-height) !important;
  padding: 0 20px !important;
  border-bottom: 1px solid var(--sb-border-soft);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.sb-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
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
.sb-menu-toggle {
  font-size: 18px;
}
.sb-mock-tag {
  margin-inline-end: 0;
  font-weight: 600;
}
.sb-content {
  margin: 18px;
  padding: 0;
}
.sb-drawer :deep(.ant-drawer-body) {
  padding: 0;
}

@media (max-width: 768px) {
  .sb-header {
    padding: 0 12px !important;
  }
  .sb-header-right {
    gap: 10px;
  }
  .sb-content {
    margin: 10px;
  }
}
</style>
