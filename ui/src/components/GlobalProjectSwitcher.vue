<template>
  <div class="sb-project-switcher">
    <a-dropdown
      v-model:open="open"
      trigger="click"
      overlay-class-name="sb-project-dropdown"
      :get-popup-container="popupContainer"
      @openChange="onOpenChange"
    >
      <button type="button" class="sb-project-trigger" aria-label="切换项目">
        <FolderOutlined class="sb-project-trigger__icon" />
        <span class="sb-project-trigger__text">
          <span class="sb-project-trigger__name">{{ store.displayName }}</span>
          <span v-if="store.id" class="sb-project-trigger__id">{{ shortId(store.id) }}</span>
        </span>
        <DownOutlined class="sb-project-trigger__caret" />
      </button>
      <template #overlay>
        <div class="sb-project-panel">
          <a-input
            v-model:value="query"
            allow-clear
            placeholder="搜索项目名称或 ID"
            @pressEnter="selectFirst"
          >
            <template #prefix><SearchOutlined /></template>
          </a-input>
          <div class="sb-project-list">
            <button
              v-for="p in filtered"
              :key="p.id"
              type="button"
              class="sb-project-item"
              :class="{ 'is-active': p.id === store.id }"
              @click="select(p)"
            >
              <span class="sb-project-item__name">{{ p.name || p.id }}</span>
              <span class="sb-project-item__id">{{ p.id }}</span>
            </button>
            <div v-if="!filtered.length" class="sb-project-empty">
              {{ store.loading ? '加载项目…' : '没有匹配的项目' }}
            </div>
          </div>
          <div class="sb-project-footer">
            <a-button type="link" @click="openCreate">
              <template #icon><PlusOutlined /></template>
              新建项目
            </a-button>
          </div>
        </div>
      </template>
    </a-dropdown>
    <CreateProjectModal
      :open="store.createModalOpen"
      @update:open="(v: boolean) => (v ? store.openCreateModal() : store.closeCreateModal())"
      @created="onCreated"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { DownOutlined, FolderOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { useProjectStore } from '../stores/project'
import type { ProjectItem } from '../services/types'
import CreateProjectModal from './modal/CreateProjectModal.vue'

const store = useProjectStore()
const open = ref(false)
const query = ref('')

function popupContainer() {
  return document.body
}

function shortId(id: string) {
  const compact = id.replace(/-/g, '')
  if (compact.length >= 8) return compact.slice(-8)
  return id.length > 8 ? id.slice(0, 8) : id
}

const merged = computed<ProjectItem[]>(() => {
  const map = new Map<string, ProjectItem>()
  for (const p of store.projects) {
    if (p.id) map.set(p.id, p)
  }
  for (const id of store.history) {
    if (!map.has(id)) map.set(id, { id, name: id === store.id ? store.projectName : '', createdAt: '' })
  }
  if (store.id && !map.has(store.id)) {
    map.set(store.id, { id: store.id, name: store.projectName, createdAt: '' })
  }
  return Array.from(map.values())
})

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return merged.value
  return merged.value.filter(
    (p) => p.id.toLowerCase().includes(q) || (p.name || '').toLowerCase().includes(q)
  )
})

function onOpenChange(next: boolean) {
  if (next) {
    query.value = ''
    void store.loadProjects()
  }
}

function select(p: ProjectItem) {
  if (p.id !== store.id) {
    store.setProject(p.id, p.name)
    message.success(`已切换到「${p.name || p.id}」`)
  }
  open.value = false
}

function selectFirst() {
  const first = filtered.value[0]
  if (first) select(first)
}

async function openCreate() {
  open.value = false
  await nextTick()
  store.openCreateModal()
}

async function onCreated(p: ProjectItem) {
  store.setProject(p.id, p.name)
  await store.loadProjects()
}

watch(
  () => store.createModalOpen,
  (v) => {
    if (v) open.value = false
  }
)

onMounted(() => {
  void store.loadProjects()
})
</script>

<style scoped>
.sb-project-trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--sb-space-2);
  max-width: 240px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--sb-border-soft);
  border-radius: var(--sb-radius-sm);
  background: var(--sb-surface);
  color: var(--sb-text);
  cursor: pointer;
  transition: var(--sb-transition);
}
.sb-project-trigger:hover,
.sb-project-trigger:focus-visible {
  border-color: var(--sb-primary);
  color: var(--sb-primary);
}
.sb-project-trigger__icon,
.sb-project-trigger__caret {
  color: var(--sb-text-secondary);
  font-size: var(--sb-fs-sm);
  flex-shrink: 0;
}
.sb-project-trigger__text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
  line-height: var(--sb-lh-tight);
}
.sb-project-trigger__name {
  font-size: var(--sb-fs-sm);
  font-weight: 600;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sb-project-trigger__id {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-xs);
  color: var(--sb-text-muted);
}
.sb-project-panel {
  width: 300px;
  padding: var(--sb-space-2);
  background: var(--sb-surface);
  border: 1px solid var(--sb-border-soft);
  border-radius: var(--sb-radius-sm);
  box-shadow: var(--sb-shadow);
}
.sb-project-list {
  max-height: 240px;
  overflow: auto;
  margin-top: var(--sb-space-2);
}
.sb-project-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: var(--sb-radius-sm);
  background: var(--sb-surface);
  cursor: pointer;
  text-align: left;
}
.sb-project-item:hover,
.sb-project-item.is-active {
  background: var(--sb-primary-light);
}
.sb-project-item__name {
  color: var(--sb-text);
  font-size: var(--sb-fs-sm);
}
.sb-project-item__id {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-xs);
  color: var(--sb-text-muted);
  word-break: break-all;
}
.sb-project-empty {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-sm);
  text-align: center;
  padding: var(--sb-space-4) 0;
}
.sb-project-footer {
  border-top: 1px solid var(--sb-border-soft);
  margin-top: var(--sb-space-2);
  padding-top: var(--sb-space-1);
}

@media (max-width: 768px) {
  .sb-project-trigger {
    max-width: 160px;
  }
}
</style>
<style>
.sb-project-dropdown {
  z-index: 1050;
}
.sb-project-dropdown,
.sb-project-dropdown .ant-dropdown-menu,
.sb-project-dropdown .sb-project-panel {
  background: var(--sb-surface);
}
</style>
