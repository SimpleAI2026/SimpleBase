<template>
  <ProjectScope>
    <PageContainer subtitle="到点自动调用云函数导出函数；支持 cron 定时执行与固定间隔">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>定时任务列表</CardTitle>
          <CardDescription>
            共 {{ total }} 个任务 · 调度按 UTC 执行
          </CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap items-center gap-2.5 border-b py-4">
          <Button variant="outline" :disabled="loading" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            刷新
          </Button>
          <Button v-if="!isAdminProject" :disabled="loading" @click="openCreate">
            <PlusIcon data-icon="inline-start" />
            新建定时任务
          </Button>
        </CardContent>
        <div class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-[20%] min-w-40">名称</TableHead>
                <TableHead class="w-44">调度</TableHead>
                <TableHead class="w-40">目标函数</TableHead>
                <TableHead class="w-36">状态</TableHead>
                <TableHead class="w-20">启用</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableEmpty v-if="!paged.length && !loading" :colspan="6">
                <SbEmptyState
                  :title="isAdminProject ? '系统项目不支持定时任务' : '还没有定时任务'"
                  :description="isAdminProject ? '' : '新建定时任务，到点自动调用云函数'"
                  :action-text="isAdminProject ? undefined : '新建定时任务'"
                  @action="!isAdminProject && openCreate()"
                />
              </TableEmpty>
              <TableRow v-for="record in paged" :key="record.id" class="hover:bg-muted/40">
                <TableCell>
                  <TooltipProvider :delay-duration="200">
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <span class="inline-flex flex-col gap-0.5">
                          <span class="sb-mono font-medium">{{ record.name }}</span>
                          <span v-if="record.description" class="truncate text-xs text-muted-foreground">
                            {{ record.description }}
                          </span>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>{{ record.description || record.name }}</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </TableCell>
                <TableCell>
                  <div class="flex flex-col gap-0.5">
                    <span class="sb-mono text-xs">{{ scheduleText(record) }}</span>
                    <span class="text-xs text-muted-foreground">
                      {{ record.scheduleKind === 'cron' ? '定时执行' : '固定间隔' }}
                      <template v-if="record.enabled && record.nextRunAt">
                        · 下次 {{ formatTime(record.nextRunAt) }}
                      </template>
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <span
                    class="sb-mono inline-flex items-center gap-1.5 text-sm"
                    :class="record.targetMissing ? 'text-destructive' : ''"
                  >
                    <template v-if="record.targetMissing">
                      <TooltipProvider :delay-duration="200">
                        <Tooltip>
                          <TooltipTrigger as-child>
                            <span class="inline-flex items-center gap-1.5">
                              <AlertTriangleIcon class="size-3.5" />
                              {{ record.funcFile }}.{{ record.funcExport }}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>目标函数缺失，执行将失败</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </template>
                    <template v-else>{{ record.funcFile }}.{{ record.funcExport }}</template>
                  </span>
                </TableCell>
                <TableCell>
                  <div class="flex flex-col gap-0.5">
                    <Badge :variant="statusVariant(record.lastStatus)" class="w-fit">
                      {{ statusText(record.lastStatus) }}
                    </Badge>
                    <span class="text-xs text-muted-foreground">
                      {{ record.lastRunAt ? formatTime(record.lastRunAt) : '未运行' }}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <Switch
                    :checked="record.enabled"
                    :disabled="isAdminProject || toggling.has(record.id)"
                    @update:checked="toggleEnabled(record)"
                  />
                </TableCell>
                <TableCell>
                  <div class="flex flex-wrap gap-1">
                    <Button variant="ghost" size="sm" :disabled="triggering.has(record.id)" @click="trigger(record)">
                      <PlayIcon data-icon="inline-start" />
                      立即执行
                    </Button>
                    <Button variant="ghost" size="sm" @click="openRuns(record)">
                      <HistoryIcon data-icon="inline-start" />
                      记录
                    </Button>
                    <Button v-if="!isAdminProject" variant="ghost" size="sm" @click="openEdit(record)">
                      <PencilIcon data-icon="inline-start" />
                      编辑
                    </Button>
                    <ConfirmAction
                      v-if="!isAdminProject"
                      :title="`确认删除 ${record.name}？历史运行记录保留但不再排期。`"
                      @confirm="remove(record)"
                    >
                      <Button variant="ghost" size="sm" class="text-destructive hover:bg-destructive/10">
                        <Trash2Icon data-icon="inline-start" />
                        删除
                      </Button>
                    </ConfirmAction>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <div class="flex items-center justify-between gap-3 border-t border-border px-4 py-3 sm:px-5">
            <TablePager
              :page="page"
              :page-size="pageSize"
              :total="total"
              :page-count="pageCount"
              @update:page="page = $event"
            />
          </div>
        </div>
      </Card>

      <CronJobModal v-model:open="modalOpen" :target="modalTarget" @saved="load" />
      <CronJobRunsDrawer
        v-model:open="runsOpen"
        :job="runsJob"
        @triggered="load"
      />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
/**
 * 定时任务列表页（ui-cronjob-plan §7.2）。
 * 调度/目标/状态快照/启用 Switch；立即执行触发后刷新；删除走 ConfirmAction。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import {
  AlertTriangleIcon,
  HistoryIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
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
import type { CronJobItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import CronJobModal from '../components/modal/CronJobModal.vue'
import CronJobRunsDrawer from '../components/CronJobRunsDrawer.vue'

const ADMIN_PROJECT_ID = '00000000-0000-0000-0000-000000000099'

const projectStore = useProjectStore()
const projectId = computed(() => projectStore.projectId)
const isAdminProject = computed(() => projectId.value === ADMIN_PROJECT_ID)

const records = ref<CronJobItem[]>([])
const loading = ref(false)
const toggling = ref(new Set<string>())
const triggering = ref(new Set<string>())

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)

async function load() {
  loading.value = true
  try {
    records.value = await api.cronjobs.list(projectId.value)
    page.value = 1
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载定时任务列表失败')
  } finally {
    loading.value = false
  }
}

/** 调度人类化：cron 原样等宽；interval 转「每 N 分钟/小时/天」 */
function scheduleText(record: CronJobItem): string {
  if (record.scheduleKind === 'cron') return record.cronExpr
  const s = record.intervalSeconds ?? 0
  if (s % 86400 === 0) return `每 ${s / 86400} 天`
  if (s % 3600 === 0) return `每 ${s / 3600} 小时`
  if (s % 60 === 0) return `每 ${s / 60} 分钟`
  return `每 ${s} 秒`
}

function statusVariant(status: string) {
  if (status === 'completed') return 'success' as const
  if (status === 'failed') return 'destructive' as const
  if (status === 'running') return 'secondary' as const
  return 'outline' as const
}

function statusText(status: string) {
  if (status === 'completed') return '成功'
  if (status === 'failed') return '失败'
  if (status === 'running') return '执行中'
  return '未运行'
}

async function toggleEnabled(record: CronJobItem) {
  toggling.value.add(record.id)
  try {
    const updated = await api.cronjobs.update(projectId.value, record.id, { enabled: !record.enabled })
    Object.assign(record, updated)
    toast.success(updated.enabled ? `已启用 ${record.name}` : `已暂停 ${record.name}`)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '切换失败')
  } finally {
    toggling.value.delete(record.id)
  }
}

async function trigger(record: CronJobItem) {
  triggering.value.add(record.id)
  try {
    await api.cronjobs.trigger(projectId.value, record.id)
    toast.success(`已触发 ${record.name}`)
    openRuns(record)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '触发失败')
  } finally {
    triggering.value.delete(record.id)
  }
}

async function remove(record: CronJobItem) {
  try {
    await api.cronjobs.remove(projectId.value, record.id)
    toast.success(`已删除 ${record.name}`)
    await load()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
  }
}

/* ---------- 弹窗与抽屉 ---------- */

const modalOpen = ref(false)
const modalTarget = ref<CronJobItem>()

function openCreate() {
  modalTarget.value = undefined
  modalOpen.value = true
}
function openEdit(record: CronJobItem) {
  modalTarget.value = record
  modalOpen.value = true
}

const runsOpen = ref(false)
const runsJob = ref<CronJobItem>()

function openRuns(record: CronJobItem) {
  runsJob.value = record
  runsOpen.value = true
}

onMounted(load)
watch(projectId, load)
</script>
