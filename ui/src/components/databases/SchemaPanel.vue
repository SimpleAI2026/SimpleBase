<template>
  <div class="space-y-4" data-testid="schema-panel">
    <p v-if="loading" class="text-sm text-muted-foreground">加载表结构</p>
    <template v-else>
      <SbEmptyState v-if="!tables.length" description="还没有表" />
      <div v-else class="space-y-3">
        <section v-for="table in tables" :key="table.name" class="rounded-lg border border-border p-3">
          <h3 class="text-sm font-medium">{{ table.name }}</h3>
          <ul class="mt-2 space-y-1 text-sm">
            <li v-for="col in table.columns" :key="col.name">
              <span class="sb-mono">{{ col.name }}</span>
              <span class="text-muted-foreground"> {{ col.type }}</span>
              <span v-if="col.nullable === false" class="text-muted-foreground"> NOT NULL</span>
            </li>
          </ul>
        </section>
      </div>
      <div v-if="!readonly" class="grid gap-4 border-t border-border pt-4 md:grid-cols-2">
        <form class="create-table space-y-2" @submit.prevent="submitTable">
          <h4 class="text-sm font-medium">新建表</h4>
          <Input id="schema-table-name" v-model="tableName" placeholder="表名" />
          <div v-for="(draft, index) in drafts" :key="index" class="flex flex-wrap items-center gap-2">
            <Input :id="'schema-col-' + index" v-model="draft.name" placeholder="字段名" class="min-w-28 flex-1" />
            <select :id="'schema-type-' + index" v-model="draft.type" :class="selectClass">
              <option v-for="item in columnTypes" :key="item" :value="item">{{ item }}</option>
            </select>
            <label class="flex items-center gap-1 text-xs text-muted-foreground">
              <input :id="'schema-null-' + index" v-model="draft.nullable" type="checkbox" />
              可空
            </label>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" @click="addDraft">添加字段</Button>
            <Button type="submit" size="sm" :disabled="busy">新建表</Button>
          </div>
        </form>
        <form class="add-column space-y-2" @submit.prevent="submitColumn">
          <h4 class="text-sm font-medium">添加列</h4>
          <select id="schema-add-table" v-model="addTable" :class="selectClass" :disabled="!tables.length">
            <option v-for="table in tables" :key="table.name" :value="table.name">{{ table.name }}</option>
          </select>
          <Input id="schema-add-name" v-model="addName" placeholder="字段名" :disabled="!tables.length" />
          <select id="schema-add-type" v-model="addType" :class="selectClass" :disabled="!tables.length">
            <option v-for="item in columnTypes" :key="item" :value="item">{{ item }}</option>
          </select>
          <label class="flex items-center gap-1 text-xs text-muted-foreground">
            <input id="schema-add-nullable" v-model="addNullable" type="checkbox" :disabled="!tables.length" />
            可空
          </label>
          <Button type="submit" size="sm" :disabled="busy || !tables.length">添加列</Button>
        </form>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/services/api'
import type { DatabaseItem, SchemaColumn, SchemaTable } from '@/services/api'
import { errorMessage } from '@/utils/format'
import SbEmptyState from '../SbEmptyState.vue'

const columnTypes = ['INTEGER', 'BIGINT', 'DOUBLE', 'VARCHAR', 'BOOLEAN', 'TIMESTAMP', 'DATE', 'JSON']
const selectClass =
  'border-input bg-background text-foreground h-8.5 min-w-28 rounded-lg border px-2 text-sm'
const maxDrafts = 16

const props = defineProps<{
  projectId: string
  database: DatabaseItem
  readonly?: boolean
}>()

interface ColumnDraft {
  name: string
  type: string
  nullable: boolean
}

const loading = ref(true)
const busy = ref(false)
const tables = ref<SchemaTable[]>([])
const tableName = ref('')
const drafts = ref<ColumnDraft[]>([{ name: '', type: 'VARCHAR', nullable: true }])
const addTable = ref('')
const addName = ref('')
const addType = ref('VARCHAR')
const addNullable = ref(true)

function blankDraft(): ColumnDraft {
  return { name: '', type: 'VARCHAR', nullable: true }
}

async function load() {
  loading.value = true
  try {
    const schema = await api.databases.schema(props.projectId, props.database.id)
    tables.value = schema.tables
    if (schema.tables.length > 0) {
      addTable.value = schema.tables[0].name
    } else {
      addTable.value = ''
    }
  } catch (error) {
    toast.error(errorMessage(error, '加载表结构失败'))
  } finally {
    loading.value = false
  }
}

function addDraft() {
  if (drafts.value.length >= maxDrafts) {
    toast.warning('最多 16 个字段')
    return
  }
  drafts.value.push(blankDraft())
}

async function submitTable() {
  const name = tableName.value.trim()
  if (!name) {
    toast.warning('请输入表名')
    return
  }
  const columns: SchemaColumn[] = []
  for (const draft of drafts.value) {
    const columnName = draft.name.trim()
    if (!columnName) {
      toast.warning('请输入字段名')
      return
    }
    columns.push({ name: columnName, type: draft.type, nullable: draft.nullable })
  }
  busy.value = true
  try {
    await api.databases.createTable(props.projectId, props.database.id, { name, columns })
    toast.success(`表 ${name} 已创建`)
    tableName.value = ''
    drafts.value = [blankDraft()]
    await load()
  } catch (error) {
    toast.error(errorMessage(error, '新建表失败'))
  } finally {
    busy.value = false
  }
}

async function submitColumn() {
  if (!tables.value.length) return
  const name = addName.value.trim()
  if (!name) {
    toast.warning('请输入字段名')
    return
  }
  busy.value = true
  try {
    await api.databases.addColumn(props.projectId, props.database.id, {
      table: addTable.value,
      name,
      type: addType.value,
      nullable: addNullable.value
    })
    toast.success(`字段 ${name} 已添加`)
    addName.value = ''
    await load()
  } catch (error) {
    toast.error(errorMessage(error, '添加列失败'))
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  void load()
})

watch(
  () => props.database.id,
  () => {
    void load()
  }
)
</script>
