<template>
  <ProjectScope>
    <PageContainer subtitle="管理项目内的 Go 源文件；导出的大写函数可通过 HTTP+JSON 调用">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>云函数列表</CardTitle>
          <CardDescription>
            共 {{ total }} 个文件 · 调用前缀
            <code class="sb-mono text-xs">POST /go/{{ projectId }}/{name}/{FunctionName}</code>
          </CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap items-center gap-2.5 border-b py-4">
          <Button variant="outline" size="sm" :disabled="loading" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            刷新
          </Button>
          <Button v-if="!isAdminProject" size="sm" :disabled="loading" @click="openCreate">
            <PlusIcon data-icon="inline-start" />
            新建云函数
          </Button>
        </CardContent>
        <div class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-[22%] min-w-40">文件</TableHead>
                <TableHead>导出函数</TableHead>
                <TableHead class="w-44">更新时间</TableHead>
                <TableHead class="w-56">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-if="loading && !records.length">
                <TableRow v-for="n in 3" :key="'sk-' + n">
                  <TableCell colspan="4"><Skeleton class="h-8 w-full" /></TableCell>
                </TableRow>
              </template>
              <TableEmpty v-else-if="!paged.length" :colspan="4">
                <SbEmptyState
                  :title="isAdminProject ? '系统项目不支持云函数' : '还没有云函数'"
                  :description="isAdminProject ? '系统项目不提供此功能' : '新建一个 .go 文件，导出大写函数后即可 HTTP 调用'"
                />
              </TableEmpty>
              <TableRow v-for="record in paged" :key="record.id">
                <TableCell>
                  <TooltipProvider :delay-duration="200">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <span class="sb-mono inline-flex items-center gap-2 font-medium">
                          <CodeIcon class="size-4 shrink-0 text-primary" />
                          <span class="truncate">{{ record.file }}</span>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent class="sb-mono">{{ record.name }}</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </TableCell>
                <TableCell>
                  <TooltipProvider v-if="record.exports.length" :delay-duration="200">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <div class="line-clamp-1 max-w-md">
                          <Badge
                            v-for="fn in record.exports"
                            :key="fn"
                            variant="secondary"
                            class="sb-mono mr-1.5 cursor-pointer hover:bg-secondary/70"
                            title="点击复制完整调用路径"
                            @click="copyInvokePath(record, fn)"
                          >
                            {{ fn }}
                          </Badge>
                        </div>
                      </TooltipTrigger>
                      <TooltipContent class="max-w-sm">{{ record.exports.join('、') }}</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                  <span v-else class="text-sm text-muted-foreground">—</span>
                </TableCell>
                <TableCell class="text-xs text-muted-foreground">
                  {{ formatTime(record.updatedAt) }}
                </TableCell>
                <TableCell>
                  <div class="flex gap-1">
                    <Button variant="ghost" size="sm" @click="openView(record)">
                      <EyeIcon data-icon="inline-start" />
                      查看
                    </Button>
                    <Button v-if="!isAdminProject" variant="ghost" size="sm" @click="openEdit(record)">
                      <PencilIcon data-icon="inline-start" />
                      编辑
                    </Button>
                    <Button variant="ghost" size="sm" @click="copyInvokePath(record, record.exports[0])">
                      <CopyIcon data-icon="inline-start" />
                      复制路径
                    </Button>
                    <ConfirmAction
                      v-if="!isAdminProject"
                      :title="`确认删除 ${record.file}？已导出的 ${record.exports.join(' / ') || '函数'} 将立即不可调用。`"
                      @confirm="remove(record)"
                    >
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

      <GoFunctionModal v-model:open="modalOpen" :mode="modalMode" :target="modalTarget" @saved="load" />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
/**
 * 云函数列表页（ui-gofunction-plan §8.3）。
 * Badge 点击复制完整调用 URL；删除走 ConfirmAction；项目切换重载。
 */
import { onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import {
  CodeIcon,
  CopyIcon,
  EyeIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '../services/api'
import type { GoFunctionItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import GoFunctionModal from '../components/modal/GoFunctionModal.vue'

const projectStore = useProjectStore()
const { projectId, isAdmin: isAdminProject } = storeToRefs(projectStore)

const records = ref<GoFunctionItem[]>([])
const loading = ref(false)

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)

async function load() {
  loading.value = true
  try {
    records.value = await api.gofunctions.list(projectId.value)
    page.value = 1
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载云函数列表失败')
  } finally {
    loading.value = false
  }
}

/* ---------- 弹窗 ---------- */

const modalOpen = ref(false)
const modalMode = ref<'create' | 'edit' | 'view'>('create')
const modalTarget = ref<{ name: string; source?: string }>()

function openCreate() {
  modalMode.value = 'create'
  modalTarget.value = undefined
  modalOpen.value = true
}
function openEdit(record: GoFunctionItem) {
  modalMode.value = 'edit'
  modalTarget.value = { name: record.name }
  modalOpen.value = true
}
function openView(record: GoFunctionItem) {
  modalMode.value = 'view'
  modalTarget.value = { name: record.name }
  modalOpen.value = true
}

async function remove(record: GoFunctionItem) {
  try {
    await api.gofunctions.remove(projectId.value, record.name)
    toast.success(`已删除 ${record.file}`)
    await load()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
  }
}

function copyInvokePath(record: GoFunctionItem, fn?: string) {
  if (!fn) {
    toast.error('该云函数没有导出函数')
    return
  }
  const path = `POST /go/${projectId.value}/${record.name}/${fn}`
  navigator.clipboard
    .writeText(path)
    .then(() => toast.success(`已复制 ${path}`))
    .catch(() => toast.error('复制失败'))
}

onMounted(load)
watch(projectId, load)
</script>
