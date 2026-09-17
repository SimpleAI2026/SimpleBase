<template>
  <ProjectScope>
  <PageContainer title="数据库管理" subtitle="DuckLake 数据库、SQL 工作台与集合文档">
    <Card>
      <CardContent class="flex flex-wrap items-center gap-3">
        <Button variant="outline" :disabled="loading" @click="doLoad">
          <Spinner v-if="loading" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
        <Button @click="openCreate">
          <PlusIcon data-icon="inline-start" />
          新建数据库
        </Button>
        <span class="w-full text-xs text-muted-foreground sm:ml-auto sm:w-auto">共 {{ databases.length }} 个数据库</span>
      </CardContent>
    </Card>

    <Card>
      <CardHeader class="border-b">
        <CardTitle>数据库列表</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-8" />
              <TableHead class="w-40">名称</TableHead>
              <TableHead class="w-36">ID</TableHead>
              <TableHead class="w-24">状态</TableHead>
              <TableHead class="w-44">创建时间</TableHead>
              <TableHead>操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableEmpty v-if="!paged.length && !loading" :colspan="6">
              <SbEmptyState description="暂无数据库" action-text="新建数据库" @action="openCreate" />
            </TableEmpty>
            <template v-for="record in paged" :key="record.id">
              <TableRow>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <Button
                        variant="outline"
                        size="icon-xs"
                        class="rounded-full"
                        :disabled="!isReady(record)"
                        @click="toggleExpand(record)"
                      >
                        <MinusIcon v-if="expandedRowKeys.includes(record.id)" />
                        <PlusIcon v-else />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      {{ isReady(record) ? (expandedRowKeys.includes(record.id) ? '收起集合' : '展开集合') : '数据库未就绪' }}
                    </TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <span class="sb-mono inline-flex items-center gap-1">
                    <DatabaseIcon /> {{ record.name }}
                  </span>
                </TableCell>
                <TableCell>
                  <Tooltip>
                    <TooltipTrigger as-child>
                      <span class="sb-mono block max-w-36 truncate text-muted-foreground">{{ record.id }}</span>
                    </TooltipTrigger>
                    <TooltipContent>{{ record.id }}</TooltipContent>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <Badge :variant="statusBadgeVariant(record.status)">{{ statusText(record.status) }}</Badge>
                </TableCell>
                <TableCell>{{ formatTime(record.createdAt) }}</TableCell>
                <TableCell>
                  <div class="flex flex-wrap items-center gap-1">
                    <Button variant="ghost" size="sm" :disabled="!isOpenable(record)" @click="openDb(record)">
                      <RocketIcon data-icon="inline-start" />
                      打开
                    </Button>
                    <Button variant="ghost" size="sm" :disabled="!isReady(record)" @click="closeDb(record)">
                      <PowerIcon data-icon="inline-start" />
                      关闭
                    </Button>
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <span>
                          <Button variant="ghost" size="sm" :disabled="!isReady(record)" @click="openSql(record)">SQL</Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>{{ isReady(record) ? 'SQL 工作台' : '数据库未就绪' }}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <span>
                          <Button variant="ghost" size="sm" :disabled="!isReady(record)" @click="openCreateCollection(record)">新建集合</Button>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>{{ isReady(record) ? '新建集合' : '数据库未就绪' }}</TooltipContent>
                    </Tooltip>
                    <ConfirmAction
                      :disabled="record.status === 'deleting'"
                      title="删除为异步操作，确认继续？"
                      @confirm="removeDb(record)"
                    >
                      <Button variant="ghost" size="sm" class="text-destructive" :disabled="record.status === 'deleting'">
                        <Trash2Icon data-icon="inline-start" />
                        删除
                      </Button>
                    </ConfirmAction>
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-if="expandedRowKeys.includes(record.id)">
                <TableCell colspan="6">
                  <CollectionPanel
                    :project-id="projectStore.id"
                    :database="record"
                    :reload-token="collectionReload[record.id] || 0"
                    @view-data="(c) => openDocList(record, c)"
                    @add-document="(c) => openKv(record, c)"
                    @create-collection="openCreateCollection(record)"
                  />
                </TableCell>
              </TableRow>
            </template>
          </TableBody>
        </Table>
        <TablePager
          :page="page"
          :page-size="pageSize"
          :total="total"
          :page-count="pageCount"
          @update:page="page = $event"
        />
      </CardContent>
    </Card>

    <SbModal
      v-model:open="createVisible"
      title="新建数据库"
      :confirm-loading="creating"
      :ok-button-props="{ disabled: !!nameError && !!newName.trim() }"
      @ok="create"
    >
      <Field :data-invalid="nameError ? true : undefined">
        <FieldLabel for="db-name">名称</FieldLabel>
        <Input
          id="db-name"
          v-model="newName"
          placeholder="名称（1-63 字节，不含 / \\ 与控制字符）"
          :aria-invalid="nameError ? true : undefined"
        />
        <FieldDescription v-if="nameError">{{ nameError }}</FieldDescription>
      </Field>
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
import { toast } from 'vue-sonner'
import {
  DatabaseIcon,
  MinusIcon,
  PlusIcon,
  PowerIcon,
  RefreshCwIcon,
  RocketIcon,
  Trash2Icon,
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '../services/api'
import type { DatabaseItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useAsyncAction } from '../composables/useAsyncAction'
import { usePagination } from '../composables/usePagination'
import { statusBadgeVariant, statusText } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import SbModal from '../components/modal/SbModal.vue'
import SqlWorkModal from '../components/modal/SqlWorkModal.vue'
import CreateCollectionModal from '../components/modal/CreateCollectionModal.vue'
import DocumentListModal from '../components/modal/DocumentListModal.vue'
import DocumentKvModal from '../components/modal/DocumentKvModal.vue'
import CollectionPanel from '../components/databases/CollectionPanel.vue'

const projectStore = useProjectStore()
const databases = ref<DatabaseItem[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(databases)

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

const nameError = computed(() => {
  const n = newName.value.trim()
  if (!n) return ''
  if (n.length > 63) return '名称不能超过 63 字节'
  if (/[\/\\\x00-\x1f]/.test(n)) return '名称不能包含 / \\ 或控制字符'
  return ''
})

function isReady(db: DatabaseItem) {
  return db.status === 'ready'
}

function isOpenable(db: DatabaseItem) {
  return ['ready', 'closed', 'degraded'].includes(db.status)
}

function toggleExpand(db: DatabaseItem) {
  if (!isReady(db)) return
  if (expandedRowKeys.value.includes(db.id)) {
    expandedRowKeys.value = expandedRowKeys.value.filter((id) => id !== db.id)
  } else {
    expandedRowKeys.value = [...expandedRowKeys.value, db.id]
  }
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
    toast.warning('请输入数据库名称')
    return
  }
  if (nameError.value) {
    toast.warning(nameError.value)
    return
  }
  creating.value = true
  try {
    const db = await api.databases.create(projectStore.id, name)
    toast.success(`数据库 ${db.name} 创建成功（状态：${statusText(db.status)}）`)
    createVisible.value = false
    await doLoad()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '创建失败')
  } finally {
    creating.value = false
  }
}

async function openDb(db: DatabaseItem) {
  try {
    await api.databases.open(projectStore.id, db.id)
    toast.success(`${db.name} 已打开`)
    await doLoad()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '打开失败')
  }
}

async function closeDb(db: DatabaseItem) {
  try {
    await api.databases.close(projectStore.id, db.id)
    toast.success(`${db.name} 已关闭（数据保留）`)
    await doLoad()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '关闭失败')
  }
}

async function removeDb(db: DatabaseItem) {
  try {
    await api.databases.remove(projectStore.id, db.id)
    toast.success(`${db.name} 删除请求已提交（异步清理）`)
    await doLoad()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
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
