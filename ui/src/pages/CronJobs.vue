<template>
  <ProjectScope>
    <PageContainer subtitle="到点自动调用云函数导出函数；支持 cron 定时执行与固定间隔">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>定时任务列表</CardTitle>
          <CardDescription>
            共 {{ total }} 个任务 · 调度按 UTC 执行
          </CardDescription>
          <CardAction v-if="!isAdminProject">
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="loading" @click="load">
                <Spinner v-if="loading" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                刷新
              </Button>
              <Button size="sm" @click="openCreate">
                <PlusIcon data-icon="inline-start" />
                新建定时任务
              </Button>
            </div>
          </CardAction>
          <CardAction v-else>
            <Button variant="outline" size="sm" :disabled="loading" @click="load">
              <Spinner v-if="loading" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              刷新
            </Button>
          </CardAction>
        </CardHeader>
        <div class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="min-w-40 max-w-56">名称</TableHead>
                <TableHead class="sb-col-md">调度</TableHead>
                <TableHead class="sb-col-name">目标函数</TableHead>
                <TableHead class="sb-col-sm">状态</TableHead>
                <TableHead class="sb-col-sm">启用</TableHead>
                <TableHead class="sb-col-act min-w-56">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-if="loading && !records.length">
                <TableRow v-for="n in 3" :key="'sk-' + n">
                  <TableCell colspan="6"><Skeleton class="h-8 w-full" /></TableCell>
                </TableRow>
              </template>
              <TableEmpty v-else-if="!paged.length" :colspan="6">
                <SbEmptyState
                  :title="isAdminProject ? '系统项目不支持定时任务' : '还没有定时任务'"
                  :description="isAdminProject ? '系统项目不提供此功能' : '新建定时任务，到点自动调用云函数'"
                  :action-text="isAdminProject ? undefined : '新建定时任务'"
                  @action="!isAdminProject && openCreate()"
                />
              </TableEmpty>
              <TableRow v-for="record in paged" :key="record.id">
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
                      {{ scheduleKindText(record.scheduleKind) }}
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
                              <AlertTriangleIcon class="size-3.5 text-destructive" />
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
                    <Badge :variant="cronStatusVariant(record.lastStatus)" class="w-fit">
                      {{ cronStatusText(record.lastStatus) }}
                    </Badge>
                    <span class="text-xs text-muted-foreground">
                      {{ record.lastRunAt ? formatTime(record.lastRunAt) : '未运行' }}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <Switch
                    :model-value="record.enabled"
                    :disabled="isAdminProject || toggling.has(record.id)"
                    @update:model-value="toggleEnabled(record)"
                  />
                </TableCell>
                <TableCell>
                  <div class="flex flex-wrap gap-1">
                    <Button variant="ghost" size="sm" @click="openRuns(record, { trigger: true })">
                      立即执行
                    </Button>
                    <Button variant="ghost" size="sm" @click="openRuns(record)">
                      记录
                    </Button>
                    <Button v-if="!isAdminProject" variant="ghost" size="sm" @click="openEdit(record)">
                      编辑
                    </Button>
                    <ConfirmAction
                      v-if="!isAdminProject"
                      :title="`确认删除 ${record.name}？历史运行记录保留但不再排期。`"
                      @confirm="remove(record)"
                    >
                      <Button variant="destructiveGhost" size="sm">
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

      <CronJobModal v-model:open="modalOpen" :target="modalTarget" @saved="load" />
      <CronJobRunsModal
        v-model:open="runsOpen"
        :job="runsJob"
        :auto-trigger="runsAutoTrigger"
        @triggered="load"
      />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
/**
 * 定时任务列表页（ui-cronjob-plan §7.2）。
 * 调度/目标/状态快照/启用 Switch；立即执行触发后刷新；删除走 ConfirmAction。
 */
import { onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import {
  AlertTriangleIcon,
  PlusIcon,
  RefreshCwIcon
} from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
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
import { useAsyncAction } from '../composables/useAsyncAction'
import { usePagination } from '../composables/usePagination'
import { cronStatusText, cronStatusVariant } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import CronJobModal from '../components/modal/CronJobModal.vue'
import CronJobRunsModal from '../components/modal/CronJobRunsModal.vue'

const projectStore = useProjectStore()
const { projectId, isAdmin: isAdminProject } = storeToRefs(projectStore)

const records = ref<CronJobItem[]>([])
const toggling = ref(new Set<string>())
const triggering = ref(new Set<string>())

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const { run: load, loading } = useAsyncAction(() => api.cronjobs.list(projectId.value), {
  fallbackMsg: '加载定时任务列表失败',
  onSuccess: (data) => {
    records.value = data as CronJobItem[]
    page.value = 1
  }
})

/** 调度人类化：cron 原样等宽；interval 转「每 N 分钟/小时/天」；once 显示执行时刻 */
function scheduleText(record: CronJobItem): string {
  if (record.scheduleKind === 'cron') return record.cronExpr
  if (record.scheduleKind === 'once') return record.runAt ? formatTime(record.runAt) : '—'
  const s = record.intervalSeconds ?? 0
  if (s % 86400 === 0) return `每 ${s / 86400} 天`
  if (s % 3600 === 0) return `每 ${s / 3600} 小时`
  if (s % 60 === 0) return `每 ${s / 60} 分钟`
  return `每 ${s} 秒`
}

function scheduleKindText(kind: CronJobItem['scheduleKind']): string {
  if (kind === 'cron') return '定时执行'
  if (kind === 'once') return '一次性'
  return '固定间隔'
}

async function toggleEnabled(record: CronJobItem) {
  toggling.value.add(record.id)
  try {
    const updated = await api.cronjobs.update(projectId.value, record.id, { enabled: !record.enabled })
    Object.assign(record, updated)
    toast.success(updated.enabled ? `已启用 ${record.name}` : `已暂停 ${record.name}`)
  } catch (e) {
    toast.error(errorMessage(e, '切换失败'))
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
    toast.error(errorMessage(e, '触发失败'))
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
    toast.error(errorMessage(e, '删除失败'))
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
const runsAutoTrigger = ref(false)

/** 「记录」只打开弹窗；「立即执行」打开后由弹窗内部触发并轮询 */
function openRuns(record: CronJobItem, opts?: { trigger?: boolean }) {
  runsJob.value = record
  runsAutoTrigger.value = !!opts?.trigger
  runsOpen.value = true
}

onMounted(load)
watch(projectId, load)
</script>
