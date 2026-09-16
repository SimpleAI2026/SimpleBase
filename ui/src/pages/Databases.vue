<template>
  <PageContainer title="数据库管理" subtitle="DuckLake 数据库的创建、打开/关闭与删除">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <ProjectPicker />
        <a-button :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          刷新
        </a-button>
        <a-button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          新建数据库
        </a-button>
        <span class="sb-hint">共 {{ databases.length }} 个数据库</span>
      </div>
    </a-card>

    <a-card class="sb-card" title="数据库列表">
      <a-table
        :columns="columns"
        :data-source="databases"
        :loading="loading"
        row-key="id"
        :pagination="pagination"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="sb-mono"><DatabaseOutlined /> {{ record.name }}</span>
          </template>
          <template v-else-if="column.key === 'id'">
            <a-tooltip :title="record.id">
              <span class="sb-mono sb-id">{{ record.id }}</span>
            </a-tooltip>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="statusColor(record.status)">{{ statusText(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'createdAt'">
            {{ formatTime(record.createdAt) }}
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button
              type="link"
              size="small"
              :disabled="!isOpenable(record)"
              @click="openDb(record)"
            >
              <template #icon><RocketOutlined /></template>
              打开
            </a-button>
            <a-button
              type="link"
              size="small"
              :disabled="record.status !== 'ready'"
              @click="closeDb(record)"
            >
              <template #icon><PoweroffOutlined /></template>
              关闭
            </a-button>
            <a-popconfirm
              title="删除为异步操作，确认继续？"
              :disabled="record.status === 'deleting'"
              @confirm="removeDb(record)"
            >
              <a-button type="link" danger size="small" :disabled="record.status === 'deleting'">
                <template #icon><DeleteOutlined /></template>
                删除
              </a-button>
            </a-popconfirm>
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState
            description="暂无数据库"
            action-text="新建数据库"
            @action="openCreate"
          />
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="createVisible"
      title="新建数据库"
      :confirm-loading="creating"
      @ok="create"
    >
      <a-input v-model:value="newName" placeholder="名称（1-63 字节，不含 / \\ 与控制字符）" />
      <div v-if="nameError" class="sb-name-error">{{ nameError }}</div>
    </a-modal>
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import {
  ReloadOutlined,
  PlusOutlined,
  DeleteOutlined,
  DatabaseOutlined,
  RocketOutlined,
  PoweroffOutlined
} from '@ant-design/icons-vue'
import { api } from '../services/api'
import type { DatabaseItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useAsyncAction } from '../composables/useAsyncAction'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectPicker from '../components/ProjectPicker.vue'
import SbEmptyState from '../components/SbEmptyState.vue'

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 160 },
  { title: 'ID', dataIndex: 'id', key: 'id', width: 150, ellipsis: true },
  { title: '状态', key: 'status', width: 100 },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 180 },
  { title: '操作', key: 'ops', width: 200 }
]

const projectStore = useProjectStore()
const pagination = usePagination()

const databases = ref<DatabaseItem[]>([])
const createVisible = ref(false)
const creating = ref(false)
const newName = ref('')

const { run: load, loading } = useAsyncAction(
  () => api.databases.list(projectStore.id),
  { fallbackMsg: '数据库列表加载失败' }
)
// useAsyncAction 返回 data，但表格直接绑 databases（load 后同步）
async function doLoad() {
  const list = await load()
  if (list) databases.value = list
}

const nameError = computed(() => {
  const n = newName.value.trim()
  if (!n) return ''
  if (n.length > 63) return '名称不能超过 63 字节'
  if (/[\/\\\x00-\x1f]/.test(n)) return '名称不能包含 / \\ 或控制字符'
  return ''
})

/** 状态 tag 配色：覆盖后端全部 9 个枚举值（catalog/model.go） */
const statusColorMap: Record<string, string> = {
  ready: 'success',
  creating: 'processing',
  opening: 'processing',
  closing: 'processing',
  recovering: 'processing',
  closed: 'default',
  degraded: 'warning',
  deleting: 'warning',
  deleted: 'default'
}
const statusTextMap: Record<string, string> = {
  ready: '就绪',
  creating: '创建中',
  opening: '打开中',
  closing: '关闭中',
  recovering: '恢复中',
  closed: '已关闭',
  degraded: '降级',
  deleting: '删除中',
  deleted: '已删除'
}
function statusColor(s: string) {
  return statusColorMap[s] || 'default'
}
function statusText(s: string) {
  return statusTextMap[s] || s
}

/** open 仅对已就绪/已关闭/降级的库有意义（与服务端状态机一致） */
function isOpenable(db: DatabaseItem) {
  return ['ready', 'closed', 'degraded'].includes(db.status)
}

function openCreate() {
  newName.value = ''
  createVisible.value = true
}

async function create() {
  const name = newName.value.trim()
  if (!name) {
    message.warning('请输入数据库名称')
    return
  }
  if (nameError.value) {
    message.warning(nameError.value)
    return
  }
  creating.value = true
  try {
    const db = await api.databases.create(projectStore.id, name)
    message.success(`数据库 ${db.name} 创建成功（状态：${statusText(db.status)}）`)
    createVisible.value = false
    await doLoad()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '创建失败')
  } finally {
    creating.value = false
  }
}

async function openDb(db: DatabaseItem) {
  try {
    await api.databases.open(projectStore.id, db.id)
    message.success(`${db.name} 已打开`)
    await doLoad()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '打开失败')
  }
}

async function closeDb(db: DatabaseItem) {
  try {
    await api.databases.close(projectStore.id, db.id)
    message.success(`${db.name} 已关闭（数据保留）`)
    await doLoad()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '关闭失败')
  }
}

async function removeDb(db: DatabaseItem) {
  try {
    await api.databases.remove(projectStore.id, db.id)
    message.success(`${db.name} 删除请求已提交（异步清理）`)
    await doLoad()
  } catch (e) {
    message.error(e instanceof Error ? e.message : '删除失败')
  }
}

onMounted(doLoad)
watch(() => projectStore.id, () => {
  void doLoad()
})
</script>

<style scoped>
.sb-hint {
  color: var(--sb-text-muted);
  font-size: var(--sb-fs-xs);
  margin-left: auto;
}
.sb-id {
  color: var(--sb-text-secondary);
}
.sb-name-error {
  color: var(--sb-danger);
  font-size: var(--sb-fs-xs);
  margin-top: 6px;
}
@media (max-width: 768px) {
  .sb-hint {
    margin-left: 0;
    flex: 1 1 100%;
  }
}
</style>
