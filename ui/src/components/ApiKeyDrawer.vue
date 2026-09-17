<template>
  <a-drawer
    v-model:open="open"
    title="连接设置"
    placement="right"
    :width="380"
    class="sb-key-drawer"
  >
    <a-alert
      v-if="unauthorized"
      type="error"
      show-icon
      message="API Key 无效或已过期"
      description="最近一次请求返回 401，请检查 Key 是否正确。"
      style="margin-bottom: 16px"
    />
    <a-form layout="vertical">
      <a-form-item label="当前项目" help="请在右上角切换或创建项目">
        <a-input :value="projectLabel" disabled />
      </a-form-item>
      <a-form-item label="API Key" help="存储于浏览器 localStorage，刷新后保留">
        <a-input-password v-model:value="key" placeholder="sb_live_..." @press-enter="saveKey" />
      </a-form-item>
      <a-form-item>
        <a-space>
          <a-button type="primary" @click="saveAll">保存</a-button>
          <a-button @click="resetKey">恢复默认（DevMode）</a-button>
        </a-space>
      </a-form-item>
    </a-form>
    <a-divider>说明</a-divider>
    <p class="sb-help">
      DevMode 种子 Key：<code>sb_live_dev_key_12345</code>（权限：Database Read/Write/Admin +
      LLMInvoke + ProjectAdmin）。生产环境请使用管理端签发的 Key。
    </p>
  </a-drawer>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { useAuthStore } from '../stores/auth'
import { useProjectStore } from '../stores/project'
import { getApiKey } from '../services/http'

const authStore = useAuthStore()
const projectStore = useProjectStore()

const open = computed({
  get: () => authStore.keyDrawerOpen,
  set: (v) => (v ? authStore.openDrawer() : authStore.closeDrawer())
})

const unauthorized = computed(() => authStore.lastUnauthorizedAt > 0)

const key = ref(authStore.apiKey)
const projectLabel = computed(() => {
  const name = projectStore.displayName
  const id = projectStore.id
  if (!id) return '未选择'
  return name && name !== id ? `${name}（${id}）` : id
})

watch(open, (v) => {
  if (v) {
    key.value = getApiKey()
  }
})

function saveKey() {
  if (key.value.trim()) authStore.updateKey(key.value)
}

function saveAll() {
  saveKey()
  message.success('设置已保存')
  authStore.closeDrawer()
}

function resetKey() {
  key.value = 'sb_live_dev_key_12345'
  authStore.updateKey(key.value)
  message.success('已恢复 DevMode 默认 Key')
}
</script>
<style scoped>
.sb-help {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
  line-height: var(--sb-lh-relaxed);
}
.sb-help code {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-xs);
  background: var(--sb-bg-soft);
  padding: 1px 4px;
  border-radius: var(--sb-radius-xs);
}
</style>
