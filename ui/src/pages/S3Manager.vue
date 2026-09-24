<template>
  <ProjectScope>
  <PageContainer subtitle="对象的上传、浏览与删除">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>对象列表</CardTitle>
        <CardDescription>按前缀筛选，支持上传与删除</CardDescription>
      </CardHeader>
      <CardContent class="flex flex-col gap-3 border-b py-4">
        <div class="flex flex-wrap items-center gap-2.5">
          <InputGroup class="min-w-56 flex-1 sm:max-w-80">
            <InputGroupAddon>
              <SearchIcon />
            </InputGroupAddon>
            <InputGroupInput
              v-model="prefix"
              placeholder="前缀筛选，如 images/"
              @keydown.enter="load"
            />
          </InputGroup>
          <Button variant="outline" size="sm" :disabled="loading" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            刷新
          </Button>
          <input ref="fileInput" type="file" class="hidden" @change="onFileChange" />
          <Button size="sm" :disabled="uploading" @click="triggerUpload">
            <Spinner v-if="uploading" data-icon="inline-start" />
            <UploadIcon v-else data-icon="inline-start" />
            上传对象
          </Button>
        </div>
        <Progress v-if="uploading && uploadPercent > 0" :model-value="uploadPercent" class="w-full" />
      </CardContent>
      <div class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="max-w-lg">Key</TableHead>
              <TableHead class="sb-col-sm">大小</TableHead>
              <TableHead class="sb-col-md">修改时间</TableHead>
              <TableHead class="w-32">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <template v-if="loading && !objects.length">
              <TableRow v-for="n in 3" :key="'sk-' + n">
                <TableCell colspan="4"><Skeleton class="h-8 w-full" /></TableCell>
              </TableRow>
            </template>
            <TableEmpty v-else-if="!paged.length" :colspan="4">
              <SbEmptyState description="暂无对象" action-text="上传对象" @action="triggerUpload" />
            </TableEmpty>
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
                <div class="flex gap-1">
                  <Button variant="ghost" size="sm" @click="open(record.key)">
                    <EyeIcon data-icon="inline-start" />
                    打开
                  </Button>
                  <ConfirmAction title="确认删除该对象？" @confirm="remove(record.key)">
                    <Button variant="destructiveGhost" size="sm">
                      <Trash2Icon data-icon="inline-start" />
                      删除
                    </Button>
                  </ConfirmAction>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
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
  </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import axios from 'axios'
import { EyeIcon, FileIcon, RefreshCwIcon, SearchIcon, Trash2Icon, UploadIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
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
import { api, isMock } from '../services/api'
import type { S3Object } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { formatBytes, formatTime } from '../utils/format'
import { getApiKey } from '../services/http'
import { baseURL } from '../services/http'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'

const projectStore = useProjectStore()
const prefix = ref('')
const objects = ref<S3Object[]>([])
const { page, pageSize, total, pageCount, items: paged } = usePagination(objects)
const loading = ref(false)
const uploading = ref(false)
const uploadPercent = ref(0)
const fileInput = ref<HTMLInputElement | null>(null)

const MAX_UPLOAD_BYTES = isMock ? 1024 * 1024 * 1024 : 10 * 1024 * 1024

function validateKey(key: string): string {
  if (!key) return 'key 不能为空'
  if (key.length > 1024) return 'key 不能超过 1024 字节'
  if (key.includes('\0')) return 'key 不能包含 NUL 字符'
  if (key.startsWith('/') || key.includes('\\')) return 'key 必须是相对路径'
  for (const seg of key.split('/')) {
    if (seg === '..' || seg === '.') return 'key 不能包含 . 或 .. 路径段'
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
    toast.warning(`文件 ${(file.size / 1024 / 1024).toFixed(1)}MB 超过上限 ${MAX_UPLOAD_BYTES / 1024 / 1024}MB`)
    return
  }
  const err = validateKey(file.name)
  if (err) {
    toast.warning(`文件名不符合 key 规则：${err}`)
    return
  }
  doUpload(file)
}

function doUpload(file: File) {
  const pid = projectStore.id
  const fd = new FormData()
  fd.append('key', file.name)
  fd.append('file', file)
  uploading.value = true
  uploadPercent.value = 0
  if (isMock) {
    api.s3
      .upload(pid, file.name, file)
      .then(() => {
        toast.success(`${file.name} 上传成功`)
        return load()
      })
      .catch((e) => toast.error(e instanceof Error ? e.message : '上传失败'))
      .finally(() => {
        uploading.value = false
      })
    return
  }
  axios
    .post(`${baseURL}/v1/projects/${encodeURIComponent(pid)}/s3/objects`, fd, {
      headers: { Authorization: `Bearer ${getApiKey()}` },
      onUploadProgress: (e) => {
        if (e.total) uploadPercent.value = Math.round((e.loaded / e.total) * 100)
      }
    })
    .then(async () => {
      toast.success(`${file.name} 上传成功`)
      await load()
    })
    .catch((e) => {
      const msg = e?.response?.data?.error?.message || e?.message || '上传失败'
      toast.error(msg)
    })
    .finally(() => {
      uploading.value = false
    })
}

function triggerUpload() {
  fileInput.value?.click()
}

async function load() {
  loading.value = true
  try {
    objects.value = await api.s3.list(projectStore.id, prefix.value || undefined)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function remove(key: string) {
  try {
    await api.s3.remove(projectStore.id, key)
    toast.success('删除成功')
    await load()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
  }
}

async function open(key: string) {
  try {
    const { url } = await api.s3.presign(projectStore.id, key)
    window.open(url, '_blank')
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '生成链接失败')
  }
}

onMounted(load)
watch(() => projectStore.id, () => {
  void load()
})
</script>
