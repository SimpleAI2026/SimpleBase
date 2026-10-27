<template>
  <ProjectScope>
  <PageContainer :subtitle="t('pages.s3.subtitle')">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>{{ t('pages.s3.cardTitle') }}</CardTitle>
        <CardDescription>{{ t('pages.s3.cardDesc') }}</CardDescription>
        <CardAction>
          <div class="flex items-center gap-2">
            <Button variant="outline" size="sm" :disabled="pending" @click="load">
              <Spinner v-if="pending" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              {{ t('common.refresh') }}
            </Button>
            <input ref="fileInput" type="file" class="hidden" @change="onFileChange" />
            <Button size="sm" :disabled="uploading" @click="triggerUpload">
              <Spinner v-if="uploading" data-icon="inline-start" />
              <UploadIcon v-else data-icon="inline-start" />
              {{ t('pages.s3.upload') }}
            </Button>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent class="flex flex-col gap-3 border-b py-4">
        <div class="flex flex-wrap items-center gap-2.5">
          <InputGroup class="min-w-56 flex-1 sm:max-w-80">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              v-model="prefix"
              :placeholder="t('pages.s3.prefixPlaceholder')"
              @keydown.enter="load"
            />
          </InputGroup>
        </div>
        <Progress v-if="uploading && uploadPercent > 0" :model-value="uploadPercent" class="w-full" />
      </CardContent>
      <div class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="max-w-lg">{{ t('pages.s3.colFile') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.s3.size') }}</TableHead>
              <TableHead class="sb-col-md">{{ t('pages.s3.modifiedAt') }}</TableHead>
              <TableHead class="w-32">{{ t('common.actions') }}</TableHead>
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
            :empty-description="t('pages.s3.emptyDesc')"
            :empty-action-text="t('pages.s3.upload')"
            @retry="load"
            @empty-action="triggerUpload"
          >
            <TableRow v-for="folder in folders" :key="folder">
              <TableCell>
                <Button variant="ghost" size="sm" class="max-w-full justify-start" @click="openFolder(folder)">
                  <FolderIcon class="size-4 shrink-0 text-primary" /><span class="truncate">{{ folder }}</span>
                </Button>
              </TableCell>
              <TableCell colspan="3" />
            </TableRow>
            <TableRow v-for="record in paged" :key="record.key">
              <TableCell class="max-w-lg">
                <span class="sb-mono inline-flex items-center gap-2 font-medium">
                  <FileIcon class="size-4 shrink-0 text-primary" />
                  <span class="truncate" :title="record.key">{{ record.key }}</span>
                </span>
              </TableCell>
              <TableCell class="sb-col-sm text-xs text-muted-foreground">{{ formatBytes(record.size) }}</TableCell>
              <TableCell class="sb-col-md text-xs text-muted-foreground">{{ formatTime(record.lastModified) }}</TableCell>
              <TableCell class="w-32">
                <div class="flex justify-center gap-1">
                  <Button variant="ghost" size="sm" @click="openPreview(record)">{{ t('pages.s3.open') }}</Button>
                  <Button variant="ghost" size="sm" @click="download(record)">{{ t('pages.s3.download') }}</Button>
                  <Button variant="ghost" size="sm" @click="copyPrivateLink(record)">{{ t('pages.s3.privateLink') }}</Button>
                  <ConfirmAction :title="t('pages.s3.deleteConfirm')" @confirm="remove(record.key)">
                    <Button variant="destructiveGhost" size="sm">
                      {{ t('common.del') }}
                    </Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
        <TablePager
          variant="footer"
          :page="page"
          :page-size="pageSize"
          :total="total"
          :page-count="pageCount"
          @update:page="page = $event"
        />
      </div>
    </Card>
    <S3PreviewModal v-model:open="previewOpen" :project-id="projectStore.id" :record="previewTarget" />
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { FileIcon, FolderIcon, RefreshCwIcon, SearchIcon, UploadIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../services/api'
import type { S3Object } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useLoadState } from '../composables/useLoadState'
import { usePagination } from '../composables/usePagination'
import { formatBytes, formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import S3PreviewModal from '../components/modal/S3PreviewModal.vue'
import TablePager from '../components/TablePager.vue'

const { t } = useI18n()
const projectStore = useProjectStore()
const prefix = ref('')
const objects = ref<S3Object[]>([])
const folders = ref<string[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(objects)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.s3.loadFailed'
})
const uploading = ref(false)
const uploadPercent = ref(0)
const fileInput = ref<HTMLInputElement | null>(null)
const previewOpen = ref(false)
const previewTarget = ref<S3Object | null>(null)

const MAX_UPLOAD_BYTES = 50 * 1024 * 1024

function validateKey(key: string): string {
  if (!key) return t('pages.s3.keyRequired')
  if (key.length > 1024) return t('pages.s3.keyTooLong')
  if (key.includes('\0')) return t('pages.s3.keyNul')
  if (key.startsWith('/') || key.includes('\\')) return t('pages.s3.keyRelative')
  for (const seg of key.split('/')) {
    if (seg === '..' || seg === '.') return t('pages.s3.keyDotSegment')
  }
  return ''
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) handleBeforeUpload(file)
}

function handleBeforeUpload(file: File) {
  if (file.size > MAX_UPLOAD_BYTES) {
    toast.warning(
      t('pages.s3.fileTooLarge', {
        size: (file.size / 1024 / 1024).toFixed(1),
        max: MAX_UPLOAD_BYTES / 1024 / 1024
      })
    )
    return
  }
  const err = validateKey(file.name)
  if (err) {
    toast.warning(t('pages.s3.keyRuleViolation', { reason: err }))
    return
  }
  doUpload(file)
}

function doUpload(file: File) {
  const pid = projectStore.id
  uploading.value = true
  uploadPercent.value = 0
  api.s3
    .upload(pid, file.name, file, (pct) => {
      uploadPercent.value = pct
    })
    .then(async () => {
      toast.success(t('pages.s3.uploadSuccess', { name: file.name }))
      await load()
    })
    .catch((e) => toast.error(errorMessage(e, t('pages.s3.uploadFailed'))))
    .finally(() => {
      uploading.value = false
    })
}

function triggerUpload() {
  fileInput.value?.click()
}

function load() {
  return run(async () => {
    const data = await api.s3.listDetailed(projectStore.id, prefix.value || undefined)
    objects.value = data.objects
    folders.value = data.folders
    return data.objects.length + data.folders.length > 0
  })
}
function openFolder(folder: string) {
  prefix.value = folder
  void load()
}
async function download(record: S3Object) {
  try {
    const blob = await api.s3.download(projectStore.id, record.key)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = record.key.split('/').pop() || 'download'
    anchor.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.downloadFailed')))
  }
}
async function copyPrivateLink(record: S3Object) {
  try {
    const link = await api.s3.createLink(projectStore.id, record.key)
    await navigator.clipboard.writeText(new URL(link.url, window.location.origin).toString())
    toast.success(t('pages.s3.linkCopied'))
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.linkFailed')))
  }
}

async function remove(key: string) {
  try {
    await api.s3.remove(projectStore.id, key)
    toast.success(t('pages.s3.deleteSuccess'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.deleteFailed')))
  }
}

function openPreview(record: S3Object) {
  previewTarget.value = record
  previewOpen.value = true
}

void load()
watch(() => projectStore.id, () => {
  void load()
})
</script>
