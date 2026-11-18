<template>
  <ProjectScope>
  <PageContainer :subtitle="t('pages.s3.subtitle')">
    <SbListCard
      :title="t('pages.s3.cardTitle')"
      :description="t('pages.s3.cardDesc')"
      :pager="{ page, pageSize, total, pageCount }"
      @update:page="page = $event"
    >
      <template #actions>
        <SbRefreshButton :pending="pending" @click="load" />
        <input ref="fileInput" type="file" class="hidden" @change="onFileChange" />
        <Button size="sm" :disabled="uploading" @click="triggerUpload">
          <Spinner v-if="uploading" data-icon="inline-start" />
          <UploadIcon v-else data-icon="inline-start" />
          {{ t('pages.s3.upload') }}
        </Button>
      </template>
      <template #toolbar>
        <div class="flex flex-col gap-3">
        <div
          v-if="site && !projectStore.isAdmin"
          class="sb-site-panel flex flex-wrap items-center gap-2.5 rounded-md border bg-muted/30 px-3 py-2"
        >
          <GlobeIcon class="size-4 shrink-0 text-primary" />
          <div class="flex min-w-0 flex-1 flex-col">
            <span class="text-sm font-medium">{{ t('pages.s3.site.title') }}</span>
            <span v-if="site.enabled && site.baseUrl" class="sb-mono truncate text-xs text-muted-foreground">
              {{ t('pages.s3.site.url') }}：<a :href="site.baseUrl" target="_blank" rel="noopener noreferrer" class="text-primary hover:underline">{{ site.baseUrl }}</a>
            </span>
            <span v-else-if="!site.publishable" class="sb-site-reason text-xs text-muted-foreground">{{ siteReasonText }}</span>
            <span v-else class="text-xs text-muted-foreground">{{ t('pages.s3.site.desc') }}</span>
          </div>
          <template v-if="site.enabled">
            <Button v-if="site.baseUrl" variant="ghost" size="sm" @click="copyUrl(site.baseUrl)">
              <CopyIcon data-icon="inline-start" />{{ t('pages.s3.copyUrl') }}
            </Button>
            <ConfirmAction
              :title="t('pages.s3.site.confirmUnpublish')"
              :description="t('pages.s3.site.confirmUnpublishDesc')"
              :disabled="siteBusy || !canManageSite"
              @confirm="setSite(false)"
            >
              <Button variant="outline" size="sm" class="sb-site-unpublish" :disabled="siteBusy || !canManageSite">
                {{ t('pages.s3.site.unpublish') }}
              </Button>
            </ConfirmAction>
          </template>
          <ConfirmAction
            v-else
            :title="t('pages.s3.site.confirmPublish')"
            :description="t('pages.s3.site.confirmPublishDesc')"
            :disabled="siteBusy || !site.publishable || !canManageSite"
            @confirm="setSite(true)"
          >
            <Button size="sm" class="sb-site-publish" :disabled="siteBusy || !site.publishable || !canManageSite">
              <Spinner v-if="siteBusy" data-icon="inline-start" />
              <GlobeIcon v-else data-icon="inline-start" />
              {{ t('pages.s3.site.publish') }}
            </Button>
          </ConfirmAction>
        </div>
        <div class="flex flex-wrap items-center gap-2.5">
          <!-- 面包屑：进入子目录后可逐级返回（planv6.7 ui-simplify-ux-plan X4） -->
          <nav v-if="crumbs.length" :aria-label="t('pages.s3.breadcrumbAria')" class="sb-s3-crumbs flex min-w-0 flex-wrap items-center gap-0.5 text-sm">
            <Button variant="ghost" size="xs" class="sb-crumb-root" @click="goPrefix('')">{{ t('pages.s3.breadcrumbRoot') }}</Button>
            <template v-for="(c, i) in crumbs" :key="c.prefix">
              <ChevronRightIcon aria-hidden="true" class="size-3.5 shrink-0 text-muted-foreground" />
              <span v-if="i === crumbs.length - 1" class="sb-crumb-current max-w-40 truncate px-1.5 font-medium" aria-current="page">{{ c.label }}</span>
              <Button v-else variant="ghost" size="xs" class="sb-crumb max-w-40 truncate" @click="goPrefix(c.prefix)">{{ c.label }}</Button>
            </template>
          </nav>
          <InputGroup class="min-w-56 flex-1 sm:max-w-80">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              v-model="prefix"
              :placeholder="t('pages.s3.prefixPlaceholder')"
              @keydown.enter="loadNow"
            />
          </InputGroup>
        </div>
        <Progress v-if="uploading && uploadPercent > 0" :model-value="uploadPercent" class="w-full" />
        </div>
      </template>
      <div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="max-w-lg">{{ t('pages.s3.colFile') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.s3.size') }}</TableHead>
              <TableHead class="sb-col-md">{{ t('pages.s3.modifiedAt') }}</TableHead>
              <TableHead v-if="showPublicCols" class="w-24">{{ t('pages.s3.colPublic') }}</TableHead>
              <TableHead v-if="showPublicCols" class="max-w-64">{{ t('pages.s3.colPublicUrl') }}</TableHead>
              <TableHead class="sb-col-act">{{ t('common.actions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <SbAsyncRegion
            as="tbody"
            :columns="columnCount"
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
            <TableRow v-for="folder in folderRows" :key="folder.key">
              <TableCell>
                <Button variant="ghost" size="sm" class="max-w-full justify-start" @click="openFolder(folder.key)">
                  <FolderIcon class="size-4 shrink-0 text-primary" /><span class="truncate">{{ folder.key }}</span>
                </Button>
              </TableCell>
              <TableCell colspan="2" />
              <template v-if="showPublicCols">
                <TableCell class="w-24">
                  <div class="flex items-center gap-1.5" :title="publicHint(folder.publicInherited)">
                    <Switch
                      class="sb-public-switch"
                      :model-value="folder.public"
                      :disabled="!switchEnabled(folder.publicInherited) || toggling.has(folder.key)"
                      @update:model-value="togglePublic(folder.key, 'folder', $event)"
                    />
                  </div>
                </TableCell>
                <TableCell class="max-w-64">
                  <div v-if="folder.publicUrl" class="sb-public-url flex min-w-0 items-center gap-1">
                    <a :href="folder.publicUrl" target="_blank" rel="noopener noreferrer" class="sb-mono truncate text-xs text-primary hover:underline" :title="folder.publicUrl">{{ folder.publicUrl }}</a>
                    <Button variant="ghost" size="icon-sm" class="sb-copy-url shrink-0" :aria-label="t('pages.s3.copyUrl')" @click="copyUrl(folder.publicUrl)"><CopyIcon /></Button>
                  </div>
                  <span v-else class="text-xs text-muted-foreground">—</span>
                </TableCell>
              </template>
              <TableCell />
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
              <template v-if="showPublicCols">
                <TableCell class="w-24">
                  <div class="flex items-center gap-1.5" :title="publicHint(record.publicInherited)">
                    <Switch
                      class="sb-public-switch"
                      :model-value="record.public === true"
                      :disabled="!switchEnabled(record.publicInherited) || toggling.has(record.key)"
                      @update:model-value="togglePublic(record.key, 'file', $event)"
                    />
                  </div>
                </TableCell>
                <TableCell class="max-w-64">
                  <div v-if="record.publicUrl" class="sb-public-url flex min-w-0 items-center gap-1">
                    <a :href="record.publicUrl" target="_blank" rel="noopener noreferrer" class="sb-mono truncate text-xs text-primary hover:underline" :title="record.publicUrl">{{ record.publicUrl }}</a>
                    <Button variant="ghost" size="icon-sm" class="sb-copy-url shrink-0" :aria-label="t('pages.s3.copyUrl')" @click="copyUrl(record.publicUrl)"><CopyIcon /></Button>
                  </div>
                  <span v-else class="text-xs text-muted-foreground">—</span>
                </TableCell>
              </template>
              <TableCell class="sb-col-act">
                <div class="flex flex-wrap items-center justify-center gap-1">
                  <Button variant="ghost" size="sm" @click="openPreview(record)">{{ t('pages.s3.open') }}</Button>
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
      </div>
    </SbListCard>
    <S3PreviewModal v-model:open="previewOpen" :project-id="projectStore.id" :record="previewTarget" />
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { ChevronRightIcon, CopyIcon, FileIcon, FolderIcon, GlobeIcon, SearchIcon, UploadIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../services/api'
import type { S3FolderMeta, S3Object, S3SiteInfo } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useAuthStore } from '../stores/auth'
import { useLoadState } from '../composables/useLoadState'
import { usePagination } from '../composables/usePagination'
import { formatBytes, formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import SbRefreshButton from '../components/SbRefreshButton.vue'
import SbListCard from '../components/SbListCard.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import S3PreviewModal from '../components/modal/S3PreviewModal.vue'

const { t } = useI18n()
const projectStore = useProjectStore()
const authStore = useAuthStore()
const prefix = ref('')
const objects = ref<S3Object[]>([])
const folders = ref<string[]>([])
const folderMeta = ref<S3FolderMeta[]>([])
const site = ref<S3SiteInfo | null>(null)
const siteBusy = ref(false)
const toggling = ref(new Set<string>())
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
  const key = uploadKey(file.name)
  const err = validateKey(key)
  if (err) {
    toast.warning(t('pages.s3.keyRuleViolation', { reason: err }))
    return
  }
  doUpload(file, key)
}

/**
 * 上传目标键：当前浏览的是目录（前缀以 / 结尾）时落进该目录，
 * 否则与之前一致落在根下（planv6.7 ui-simplify-ux-plan B7）。
 */
function uploadKey(name: string): string {
  const dir = prefix.value.trim()
  return dir.endsWith('/') ? dir + name : name
}

function doUpload(file: File, key: string) {
  const pid = projectStore.id
  uploading.value = true
  uploadPercent.value = 0
  api.s3
    .upload(pid, key, file, (pct) => {
      uploadPercent.value = pct
    })
    .then(async () => {
      toast.success(t('pages.s3.uploadSuccess', { name: key }))
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

/** 最近一次发起查询时使用的前缀；防抖回调据此跳过重复请求。 */
let loadedPrefix = ''
let prefixTimer: ReturnType<typeof setTimeout> | null = null
const PREFIX_DEBOUNCE_MS = 300

function clearPrefixTimer() {
  if (prefixTimer != null) {
    clearTimeout(prefixTimer)
    prefixTimer = null
  }
}

/** 面包屑分段：`a/b/c.txt` 只把以 `/` 结尾的目录部分拆成可点击层级。 */
const crumbs = computed(() => {
  const raw = prefix.value.trim()
  const dirEnd = raw.lastIndexOf('/')
  if (dirEnd < 0) return []
  const parts = raw.slice(0, dirEnd).split('/').filter(Boolean)
  return parts.map((label, i) => ({ label, prefix: parts.slice(0, i + 1).join('/') + '/' }))
})

function goPrefix(next: string) {
  prefix.value = next
  loadNow()
}

/** 输入后 300ms 自动查询；Enter、点面包屑、打开目录走 loadNow 立即查询。 */
watch(prefix, () => {
  clearPrefixTimer()
  prefixTimer = setTimeout(() => {
    prefixTimer = null
    if (prefix.value !== loadedPrefix) void load()
  }, PREFIX_DEBOUNCE_MS)
})
onBeforeUnmount(clearPrefixTimer)

function loadNow() {
  clearPrefixTimer()
  return load()
}

function load() {
  if (!projectStore.id) return Promise.resolve()
  loadedPrefix = prefix.value
  return run(async () => {
    const data = await api.s3.listDetailed(projectStore.id, prefix.value || undefined)
    objects.value = data.objects
    folders.value = data.folders
    folderMeta.value = data.folderMeta ?? []
    site.value = data.site ?? null
    return data.objects.length + data.folders.length > 0
  })
}

/* ---------- planv6.5：发布网站 / 外部访问 ---------- */

/** 站点功能在实例上开启时才显示两列（sites_disabled 时后端不返回 site）。 */
const showPublicCols = computed(() => site.value !== null && !projectStore.isAdmin)
const columnCount = computed(() => (showPublicCols.value ? 6 : 4))
const canManageSite = computed(() => authStore.canWrite)
const folderRows = computed<S3FolderMeta[]>(() => {
  const meta = new Map(folderMeta.value.map((m) => [m.key, m]))
  return folders.value.map((key) => meta.get(key) ?? { key, public: false })
})
const siteReasonText = computed(() => {
  const reason = site.value?.reason
  if (reason === 'sites_disabled' || reason === 'system_project' || reason === 'project_id_not_dns_safe') {
    return t(`pages.s3.site.notPublishable.${reason}`)
  }
  return reason ?? ''
})

function switchEnabled(inherited?: string) {
  return Boolean(site.value?.enabled) && canManageSite.value && !inherited
}

function publicHint(inherited?: string) {
  if (inherited === '/') return t('pages.s3.inheritedRoot')
  if (inherited) return t('pages.s3.inheritedFrom', { folder: inherited })
  if (!site.value?.enabled) return t('pages.s3.publicNeedsSite')
  return ''
}

async function setSite(enabled: boolean) {
  siteBusy.value = true
  try {
    site.value = await api.s3.setSite(projectStore.id, enabled)
    toast.success(t(enabled ? 'pages.s3.site.published' : 'pages.s3.site.unpublished'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.site.failed')))
  } finally {
    siteBusy.value = false
  }
}

async function togglePublic(key: string, kind: 'file' | 'folder', value: boolean) {
  const next = new Set(toggling.value)
  next.add(key)
  toggling.value = next
  try {
    await api.s3.setPublic(projectStore.id, key, kind, value)
    toast.success(t(value ? 'pages.s3.publicOn' : 'pages.s3.publicOff'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.publicFailed')))
  } finally {
    const rest = new Set(toggling.value)
    rest.delete(key)
    toggling.value = rest
  }
}

async function copyUrl(url?: string) {
  if (!url) return
  try {
    await navigator.clipboard.writeText(url)
    toast.success(t('pages.s3.urlCopied'))
  } catch (e) {
    toast.error(errorMessage(e, t('pages.s3.linkFailed')))
  }
}
function openFolder(folder: string) {
  goPrefix(folder)
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
