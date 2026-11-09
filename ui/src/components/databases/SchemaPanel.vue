<template>
  <div class="space-y-4" data-testid="schema-panel">
    <p v-if="readonly" class="text-sm text-muted-foreground">{{ t('components.schemaPanel.readonlyHint') }}</p>
    <div class="overflow-x-auto rounded-lg border border-border/70">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="text-left">{{ t('components.schemaPanel.colTable') }}</TableHead>
            <TableHead class="text-left">{{ t('components.schemaPanel.colColumns') }}</TableHead>
            <TableHead>{{ t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="3"
          :rows="4"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-description="t('components.schemaPanel.emptyDesc')"
          @retry="load()"
        >
          <TableRow v-for="table in tables" :key="table.name">
            <TableCell class="sb-mono text-left font-medium">{{ table.name }}</TableCell>
            <TableCell class="max-w-xl truncate text-left" :title="columnSummary(table)">{{ columnSummary(table) }}</TableCell>
            <TableCell>
              <div class="flex items-center justify-center">
                <Button type="button" variant="ghost" size="sm" @click="openRows(table.name)">{{ t('components.schemaPanel.viewData') }}</Button>
              </div>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>
    <div v-if="settled && !showError && !readonly" class="grid gap-4 border-t border-border pt-4 md:grid-cols-2">
        <form class="create-table space-y-2" @submit.prevent="submitTable">
          <h4 class="text-sm font-medium">{{ t('components.schemaPanel.createTable') }}</h4>
          <Input id="schema-table-name" v-model="tableName" :placeholder="t('components.schemaPanel.tableNamePlaceholder')" />
          <div v-for="(draft, index) in drafts" :key="index" class="flex flex-wrap items-center gap-2">
            <Input
              :id="'schema-col-' + index"
              v-model="draft.name"
              :placeholder="t('components.schemaPanel.columnNamePlaceholder')"
              class="min-w-28 flex-1"
            />
            <Select v-model="draft.type">
              <SelectTrigger
                :id="'schema-type-' + index"
                class="w-32 shrink-0"
                :aria-label="t('components.schemaPanel.columnType')"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="item in columnTypes" :key="item" :value="item">{{ item }}</SelectItem>
              </SelectContent>
            </Select>
            <label class="flex items-center gap-1 text-xs text-muted-foreground">
              <input :id="'schema-null-' + index" v-model="draft.nullable" type="checkbox" />
              {{ t('components.schemaPanel.nullable') }}
            </label>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" @click="addDraft">{{ t('components.schemaPanel.addColumnDraft') }}</Button>
            <Button type="submit" size="sm" :disabled="busy">{{ t('components.schemaPanel.submitCreateTable') }}</Button>
          </div>
        </form>
        <form class="add-column space-y-2" @submit.prevent="submitColumn">
          <h4 class="text-sm font-medium">{{ t('components.schemaPanel.addColumnSection') }}</h4>
          <Select
            :model-value="addTable || undefined"
            :disabled="!tables.length"
            @update:model-value="(v: string) => (addTable = v)"
          >
            <SelectTrigger id="schema-add-table" :aria-label="t('components.schemaPanel.targetTable')">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="table in tables" :key="table.name" :value="table.name">{{ table.name }}</SelectItem>
            </SelectContent>
          </Select>
          <Input
            id="schema-add-name"
            v-model="addName"
            :placeholder="t('components.schemaPanel.columnNamePlaceholder')"
            :disabled="!tables.length"
          />
          <Select v-model="addType" :disabled="!tables.length">
            <SelectTrigger id="schema-add-type" :aria-label="t('components.schemaPanel.columnType')">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="item in columnTypes" :key="item" :value="item">{{ item }}</SelectItem>
            </SelectContent>
          </Select>
          <label class="flex items-center gap-1 text-xs text-muted-foreground">
            <input id="schema-add-nullable" v-model="addNullable" type="checkbox" :disabled="!tables.length" />
            {{ t('components.schemaPanel.nullable') }}
          </label>
          <Button type="submit" size="sm" :disabled="busy || !tables.length">{{ t('components.schemaPanel.submitAddColumn') }}</Button>
        </form>
      </div>
    <SchemaRowsModal
      v-model:open="rowsOpen"
      :project-id="projectId"
      :database-id="database.id"
      :table="activeTable"
      :readonly="readonly"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '@/services/api'
import type { DatabaseItem, SchemaColumn, SchemaTable } from '@/services/api'
import { errorMessage } from '@/utils/format'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SchemaRowsModal from './SchemaRowsModal.vue'
import { useLoadState } from '@/composables/useLoadState'

const columnTypes = ['INTEGER', 'BIGINT', 'DOUBLE', 'VARCHAR', 'BOOLEAN', 'TIMESTAMP', 'DATE', 'JSON']
const maxDrafts = 16

const { t } = useI18n()

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

const busy = ref(false)
const rowsOpen = ref(false)
const activeTable = ref('')
const { pending, settled, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.schemaPanel.loadFailed'
})
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

function columnSummary(table: SchemaTable): string {
  return table.columns
    .map((col) => {
      const parts = [col.name, col.type]
      if (col.nullable === false) parts.push('NOT NULL')
      if (col.sensitive) parts.push(t('components.schemaPanel.hidden'))
      return parts.join(' ')
    })
    .join(', ')
}

function openRows(name: string) {
  activeTable.value = name
  rowsOpen.value = true
}

function load(replace = false) {
  return run(async () => {
    const schema = await api.databases.schema(props.projectId, props.database.id)
    tables.value = schema.tables
    if (schema.tables.length > 0) {
      addTable.value = schema.tables[0].name
    } else {
      addTable.value = ''
    }
    return schema.tables.length > 0
  }, { replace })
}

function addDraft() {
  if (drafts.value.length >= maxDrafts) {
    toast.warning(t('components.schemaPanel.maxFields'))
    return
  }
  drafts.value.push(blankDraft())
}

async function submitTable() {
  const name = tableName.value.trim()
  if (!name) {
    toast.warning(t('components.schemaPanel.tableNameRequired'))
    return
  }
  const columns: SchemaColumn[] = []
  for (const draft of drafts.value) {
    const columnName = draft.name.trim()
    if (!columnName) {
      toast.warning(t('components.schemaPanel.columnNameRequired'))
      return
    }
    columns.push({ name: columnName, type: draft.type, nullable: draft.nullable })
  }
  busy.value = true
  try {
    await api.databases.createTable(props.projectId, props.database.id, { name, columns })
    toast.success(t('components.schemaPanel.tableCreated', { name }))
    tableName.value = ''
    drafts.value = [blankDraft()]
    await load()
  } catch (error) {
    toast.error(errorMessage(error, t('components.schemaPanel.createTableFailed')))
  } finally {
    busy.value = false
  }
}

async function submitColumn() {
  if (!tables.value.length) return
  const name = addName.value.trim()
  if (!name) {
    toast.warning(t('components.schemaPanel.columnNameRequired'))
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
    toast.success(t('components.schemaPanel.columnAdded', { name }))
    addName.value = ''
    await load()
  } catch (error) {
    toast.error(errorMessage(error, t('components.schemaPanel.addColumnFailed')))
  } finally {
    busy.value = false
  }
}

void load()

watch(
  () => [props.projectId, props.database.id],
  () => {
    void load(true)
  }
)
</script>
