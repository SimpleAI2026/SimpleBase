<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="text-sm text-muted-foreground">{{ t('components.backups.hint') }}</p>
      <Button size="sm" :disabled="creating || project.isAdmin" @click="create"><Spinner v-if="creating" data-icon="inline-start" /><PlusIcon v-else data-icon="inline-start" />{{ t('components.backups.create') }}</Button>
    </div>
    <div class="overflow-x-auto rounded-lg border border-border">
      <Table>
        <TableHeader><TableRow>
          <TableHead>{{ t('components.backups.createdAt') }}</TableHead>
          <TableHead>{{ t('components.backups.engine') }}</TableHead>
          <TableHead>{{ t('components.backups.size') }}</TableHead>
          <TableHead class="w-64">{{ t('common.actions') }}</TableHead>
        </TableRow></TableHeader>
        <SbAsyncRegion
          as="tbody" :columns="4" :pending="pending" :show-skeleton="showSkeleton"
          :show-empty="showEmpty" :show-error="showError" :refreshing="refreshing" :error="error"
          :empty-description="t('components.backups.empty')" @retry="load"
        >
          <TableRow v-for="item in items" :key="item.id">
            <TableCell>{{ formatDate(item.createdAt) }}</TableCell>
            <TableCell>{{ item.engine }}</TableCell>
            <TableCell>{{ formatSize(item.sizeBytes) }}</TableCell>
            <TableCell>
              <div class="flex flex-wrap gap-1">
                <Button size="sm" variant="ghost" @click="download(item)">{{ t('components.backups.download') }}</Button>
                <ConfirmAction :title="t('components.backups.restoreConfirm')" @confirm="restore(item.id)">
                  <Button size="sm" variant="ghost" :disabled="project.isAdmin">{{ t('components.backups.restore') }}</Button>
                </ConfirmAction>
                <ConfirmAction :title="t('components.backups.deleteConfirm')" @confirm="remove(item.id)">
                  <Button size="sm" variant="destructiveGhost" :disabled="project.isAdmin">{{ t('common.del') }}</Button>
                </ConfirmAction>
              </div>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { PlusIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { BackupItem } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { errorMessage } from '@/utils/format'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import { useLoadState } from '@/composables/useLoadState'

const { t } = useI18n()
const project = useProjectStore()
const items = ref<BackupItem[]>([])
const creating = ref(false)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({ fallback: 'components.backups.loadFailed' })
function load() {
  if (!project.id || project.isAdmin) {
    items.value = []
    return Promise.resolve()
  }
  return run(async () => {
    items.value = await api.backups.list(project.id)
    return items.value.length > 0
  })
}
async function create() {
  creating.value = true
  try {
    await api.backups.create(project.id)
    toast.success(t('components.backups.created'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('components.backups.createFailed')))
  } finally {
    creating.value = false
  }
}
async function restore(id: string) {
  try {
    await api.backups.restore(project.id, id)
    toast.success(t('components.backups.restored'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.backups.restoreFailed')))
  }
}
async function remove(id: string) {
  try {
    await api.backups.remove(project.id, id)
    toast.success(t('components.backups.deleted'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('components.backups.deleteFailed')))
  }
}
async function download(item: BackupItem) {
  try {
    const blob = await api.backups.download(project.id, item.id)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${item.id}.${item.engine}`
    anchor.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.error(errorMessage(e, t('components.backups.downloadFailed')))
  }
}
function formatSize(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`
  return `${(size / 1024 / 1024).toFixed(1)} MiB`
}
function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
watch(() => project.id, load, { immediate: true })
</script>
