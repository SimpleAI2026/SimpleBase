<template>
  <ProjectScope>
  <PageContainer title="数据库管理" subtitle="DuckLake 数据库、SQL 工作台与集合文档">
    <a-card class="sb-card">
      <div class="sb-toolbar">
        <a-button :loading="loading" @click="doLoad">
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
        :expandable="expandableConfig"
      >
        <template #expandIcon="{ expanded, onExpand, record }">
          <a-tooltip :title="isReady(record) ? (expanded ? '收起集合' : '展开集合') : '数据库未就绪'">
            <button
              type="button"
              class="sb-row-expand"
              :class="{ 'is-open': expanded }"
              :disabled="!isReady(record)"
              :aria-label="expanded ? '收起集合' : '展开集合'"
              @click.stop="(e) => isReady(record) && onExpand(record, e)"
            >
              <MinusOutlined v-if="expanded" />
              <PlusOutlined v-else />
            </button>
          </a-tooltip>
        </template>
        <template #expandedRowRender="{ record }">
          <CollectionPanel
            :project-id="projectStore.id"
            :database="record"
            :reload-token="collectionReload[record.id] || 0"
            @view-data="(c) => openDocList(record, c)"
            @add-document="(c) => openKv(record, c)"
            @create-collection="openCreateCollection(record)"
          />
        </template>
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
            <div class="sb-ops">
              <a-button type="link" size="small" :disabled="!isOpenable(record)" @click="openDb(record)">
                <template #icon><RocketOutlined /></template>
                打开
              </a-button>
              <a-button type="link" size="small" :disabled="!isReady(record)" @click="closeDb(record)">
                <template #icon><PoweroffOutlined /></template>
                关闭
              </a-button>
              <a-tooltip :title="isReady(record) ? 'SQL 工作台' : '数据库未就绪'">
                <span>
                  <a-button type="link" size="small" :disabled="!isReady(record)" @click="openSql(record)">
                    SQL
                  </a-button>
                </span>
              </a-tooltip>
              <a-tooltip :title="isReady(record) ? '新建集合' : '数据库未就绪'">
                <span>
                  <a-button type="link" size="small" :disabled="!isReady(record)" @click="openCreateCollection(record)">
                    新建集合
                  </a-button>
                </span>
              </a-tooltip>
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
            </div>
          </template>
        </template>
        <template #emptyText>
          <SbEmptyState description="暂无数据库" action-text="新建数据库" @action="openCreate" />
        </template>
      </a-table>
    </a-card>

    <SbModal
      v-model:open="createVisible"
      title="新建数据库"
      :confirm-loading="creating"
      :ok-button-props="{ disabled: !!nameError && !!newName.trim() }"
      @ok="create"
    >
      <a-input v-model:value="newName" placeholder="名称（1-63 字节，不含 / \\ 与控制字符）" />
      <div v-if="nameError" class="sb-name-error">{{ nameError }}</div>
    </SbModal>

    <SqlWorkModal v-model:open="sqlOpen" :project-id="projectStore.id" :database="activeDb" />
    <CreateCollectionModal
      v-model:open="createCollectionOpen"
      :project-id="projectStore.id"
      :database="activeDb"
      @created="onCollectionCreated"
    />
    <DocumentListModal
      v-model:open="docListOpen"
      :project-id="projectStore.id"
      :database-id="activeDb?.id || ''"
      :collection="activeCollection"
      :reload-token="docReload"
      @add-document="onAddDocumentFromList"
    />
    <DocumentKvModal
      v-model:open="kvOpen"
      :project-id="projectStore.id"
      :database-id="activeDb?.id || ''"
      :collection="activeCollection"
      @created="onDocumentCreated"
    />
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import {
  ReloadOutlined,
  PlusOutlined,
  MinusOutlined,
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
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import SbModal from '../components/modal/SbModal.vue'
import SqlWorkModal from '../components/modal/SqlWorkModal.vue'
import CreateCollectionModal from '../components/modal/CreateCollectionModal.vue'
import DocumentListModal from '../components/modal/DocumentListModal.vue'
import DocumentKvModal from '../components/modal/DocumentKvModal.vue'
import CollectionPanel from '../components/databases/CollectionPanel.vue'

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 160 },
  { title: 'ID', dataIndex: 'id', key: 'id', width: 150, ellipsis: true },
  { title: '状态', key: 'status', width: 100 },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 180 },
  { title: '操作', key: 'ops', width: 320 }
]

const projectStore = useProjectStore()
const pagination = usePagination()

const databases = ref<DatabaseItem[]>([])
const createVisible = ref(false)
const creating = ref(false)
const newName = ref('')
const expandedRowKeys = ref<string[]>([])
const collectionReload = reactive<Record<string, number>>({})

const sqlOpen = ref(false)
const createCollectionOpen = ref(false)
const docListOpen = ref(false)
const kvOpen = ref(false)
const activeDb = ref<DatabaseItem | null>(null)
const activeCollection = ref('')
const docReload = ref(0)

const { run: load, loading } = useAsyncAction(() => api.databases.list(projectStore.id), {
  fallbackMsg: '数据库列表加载失败'
})

async function doLoad() {
  const list = await load()
  if (list) databases.value = list
}

const expandableConfig = computed(() => ({
  expandedRowKeys: expandedRowKeys.value,
  onExpandedRowsChange: (keys: readonly (string | number)[]) => {
    expandedRowKeys.value = keys.map(String)
  },
  rowExpandable: (record: DatabaseItem) => isReady(record)
}))

const nameError = computed(() => {
  const n = newName.value.trim()
  if (!n) return ''
  if (n.length > 63) return '名称不能超过 63 字节'
  if (/[\/\\\x00-\x1f]/.test(n)) return '名称不能包含 / \\ 或控制字符'
  return ''
})

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

function isReady(db: DatabaseItem) {
  return db.status === 'ready'
}

function isOpenable(db: DatabaseItem) {
  return ['ready', 'closed', 'degraded'].includes(db.status)
}

function openCreate() {
  newName.value = ''
  createVisible.value = true
}

function openSql(db: DatabaseItem) {
  activeDb.value = db
  sqlOpen.value = true
}

function openCreateCollection(db: DatabaseItem) {
  activeDb.value = db
  createCollectionOpen.value = true
}

function openDocList(db: DatabaseItem, collection: string) {
  activeDb.value = db
  activeCollection.value = collection
  docListOpen.value = true
}

function openKv(db: DatabaseItem, collection: string) {
  activeDb.value = db
  activeCollection.value = collection
  kvOpen.value = true
}

function onCollectionCreated() {
  if (!activeDb.value) return
  const id = activeDb.value.id
  collectionReload[id] = (collectionReload[id] || 0) + 1
  if (!expandedRowKeys.value.includes(id)) {
    expandedRowKeys.value = [...expandedRowKeys.value, id]
  }
}

function onAddDocumentFromList() {
  if (activeDb.value && activeCollection.value) {
    kvOpen.value = true
  }
}

function onDocumentCreated() {
  docReload.value += 1
  if (!docListOpen.value && activeDb.value && activeCollection.value) {
    docListOpen.value = true
  }
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

watch(databases, (list) => {
  const ready = new Set(list.filter(isReady).map((d) => d.id))
  expandedRowKeys.value = expandedRowKeys.value.filter((id) => ready.has(id))
})

onMounted(doLoad)
watch(
  () => projectStore.id,
  () => {
    expandedRowKeys.value = []
    void doLoad()
  }
)
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
.sb-ops {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
}
.sb-row-expand {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid var(--sb-border);
  border-radius: var(--sb-radius-full);
  background: var(--sb-surface);
  color: var(--sb-text-secondary);
  cursor: pointer;
  line-height: 1;
}
.sb-row-expand.is-open {
  color: var(--sb-primary);
  border-color: var(--sb-primary);
}
.sb-row-expand:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
@media (max-width: 768px) {
  .sb-hint {
    margin-left: 0;
    flex: 1 1 100%;
  }
}
</style>
