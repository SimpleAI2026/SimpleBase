<template>
  <div class="py-1">
    <div class="overflow-x-auto rounded-lg border border-border/70 bg-card/60">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="min-w-44 max-w-80">{{ t('components.collectionPanel.colName') }}</TableHead>
            <TableHead class="w-24">{{ t('components.collectionPanel.fieldCount') }}</TableHead>
            <TableHead class="w-28">{{ t('components.collectionPanel.documentCount') }}</TableHead>
            <TableHead class="w-80">{{ t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="4"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-description="readonly ? t('components.collectionPanel.emptyReadonly') : t('components.collectionPanel.empty')"
          :empty-action-text="readonly ? undefined : t('components.collectionPanel.create')"
          @retry="load"
          @empty-action="emit('create-collection')"
        >
          <TableRow v-for="record in rows" :key="record.name">
            <TableCell class="sb-mono max-w-80 truncate font-medium" :title="record.name">{{ record.name }}</TableCell>
            <TableCell>{{ record.registered ? record.fieldCount : t('components.collectionPanel.schemaless') }}</TableCell>
            <TableCell>{{ record.documentCount }}</TableCell>
            <TableCell class="w-80">
              <div class="flex flex-wrap items-center justify-center gap-1">
                <Button variant="ghost" size="sm" @click="emit('view-data', record.name)">{{ t('components.collectionPanel.viewData') }}</Button>
                <Button v-if="!readonly" variant="ghost" size="sm" @click="editSchema(record.name)">{{ t('components.collectionPanel.editSchema') }}</Button>
                <Button v-if="!readonly" variant="ghost" size="sm" @click="emit('add-document', record.name)">{{ t('components.collectionPanel.addDocument') }}</Button>
              </div>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>
    <CollectionSchemaModal
      v-model:open="schemaOpen"
      :project-id="projectId"
      :database-id="database.id"
      :collection="schemaCollection"
      @saved="load"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../services/api'
import type { CollectionInfo, DatabaseItem } from '../../services/api'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import CollectionSchemaModal from './CollectionSchemaModal.vue'
import { useLoadState } from '../../composables/useLoadState'

const props = defineProps<{
  projectId: string
  database: DatabaseItem
  reloadToken?: number
  readonly?: boolean
}>()
const { t } = useI18n()
const emit = defineEmits<{
  'view-data': [collection: string]
  'add-document': [collection: string]
  'create-collection': []
}>()
const rows = ref<CollectionInfo[]>([])
const schemaOpen = ref(false)
const schemaCollection = ref('')
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.collectionPanel.loadFailed'
})
function editSchema(name: string) {
  schemaCollection.value = name
  schemaOpen.value = true
}
function load() {
  return run(async () => {
    const [names, items] = await Promise.all([
      api.db.collections(props.projectId, props.database.id),
      api.db.collectionItems(props.projectId, props.database.id)
    ])
    const byName = new Map(items.map((item) => [item.name, item]))
    rows.value = names.map((name) => byName.get(name) ?? { name, schemaMode: 'json', fieldCount: 0, documentCount: 0, registered: false })
    return names.length > 0
  })
}
watch(
  () => [props.projectId, props.database.id, props.reloadToken],
  () => { void load() },
  { immediate: true }
)
</script>
