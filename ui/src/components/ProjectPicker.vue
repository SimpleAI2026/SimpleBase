<template>
  <a-auto-complete
    v-model:value="inner"
    class="sb-project-picker"
    style="width: 220px"
    :options="options"
    :placeholder="placeholder"
    allow-clear
    @select="onSelect"
    @focus="refresh"
    @dropdown-visible-change="onDropdown"
  >
    <template #option="{ value, label }">
      <div class="sb-project-opt">
        <span class="sb-project-opt__id">{{ value }}</span>
        <span v-if="label && label !== value" class="sb-project-opt__name">{{ label }}</span>
      </div>
    </template>
    <a-input @press-enter="commit" @blur="commit">
      <template #prefix><FolderOutlined /></template>
    </a-input>
  </a-auto-complete>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { FolderOutlined } from '@ant-design/icons-vue'
import { useProjectStore } from '../stores/project'
import { api } from '../services/api'
import type { ProjectItem } from '../services/types'

defineProps<{ tooltip?: string }>()

const projectStore = useProjectStore()
const inner = ref(projectStore.projectId)
const remote = ref<ProjectItem[]>([])
const loading = ref(false)

const placeholder = computed(() => (loading.value ? '加载项目…' : '选择或输入项目 ID'))

const options = computed(() => {
  const map = new Map<string, { value: string; label: string }>()
  for (const p of remote.value) {
    if (!p.id) continue
    map.set(p.id, { value: p.id, label: p.name ? `${p.name}` : p.id })
  }
  for (const id of projectStore.history) {
    if (!map.has(id)) map.set(id, { value: id, label: id })
  }
  // 保证当前值也在列表里，方便回看
  const cur = projectStore.projectId
  if (cur && !map.has(cur)) map.set(cur, { value: cur, label: cur })
  return Array.from(map.values())
})

watch(
  () => projectStore.projectId,
  (v) => {
    inner.value = v
  }
)

async function refresh() {
  loading.value = true
  try {
    const list = await api.projects.list()
    remote.value = Array.isArray(list) ? list : []
    for (const p of remote.value) {
      if (p.id) projectStore.remember(p.id)
    }
  } catch {
    // 后端暂不可用时仍可用本地历史下拉
    remote.value = []
  } finally {
    loading.value = false
  }
}

function onDropdown(open: boolean) {
  if (open) void refresh()
}

function onSelect(v: string) {
  inner.value = v
  commit()
}

function commit() {
  const v = (inner.value || '').trim()
  if (!v) {
    inner.value = projectStore.projectId
    return
  }
  if (v !== projectStore.projectId) {
    projectStore.setProject(v)
    message.success(`已切换到项目 ${v}`)
  } else {
    projectStore.remember(v)
  }
}

onMounted(() => {
  void refresh()
})
</script>
<style scoped>
.sb-project-picker :deep(input) {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-sm);
}
.sb-project-opt {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.3;
}
.sb-project-opt__id {
  font-family: var(--sb-font-mono);
  font-size: var(--sb-fs-sm);
}
.sb-project-opt__name {
  color: var(--sb-text-secondary, #8c8c8c);
  font-size: 12px;
}
</style>
