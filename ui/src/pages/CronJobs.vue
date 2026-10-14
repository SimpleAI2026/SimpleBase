<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.cronJobs.subtitle')">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>{{ t('pages.cronJobs.cardTitle') }}</CardTitle>
          <CardDescription>{{ t('pages.cronJobs.cardDesc', { n: total }) }}</CardDescription>
          <CardAction v-if="!isAdminProject">
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="pending" @click="load">
                <Spinner v-if="pending" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                {{ t('common.refresh') }}
              </Button>
              <Button size="sm" @click="openCreate()">
                <PlusIcon data-icon="inline-start" />
                {{ t('pages.cronJobs.create') }}
              </Button>
            </div>
          </CardAction>
          <CardAction v-else>
            <Button variant="outline" size="sm" :disabled="pending" @click="load">
              <Spinner v-if="pending" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              {{ t('common.refresh') }}
            </Button>
          </CardAction>
        </CardHeader>
        <div class="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="min-w-40 max-w-56">{{ t('pages.cronJobs.colName') }}</TableHead>
                <TableHead class="sb-col-md">{{ t('pages.cronJobs.colSchedule') }}</TableHead>
                <TableHead class="sb-col-name">{{ t('pages.cronJobs.colTarget') }}</TableHead>
                <TableHead class="sb-col-sm">{{ t('pages.cronJobs.colStatus') }}</TableHead>
                <TableHead class="sb-col-sm">{{ t('pages.cronJobs.colEnabled') }}</TableHead>
                <TableHead class="sb-col-act min-w-56">{{ t('pages.cronJobs.colActions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <SbAsyncRegion
              as="tbody"
              :columns="6"
              :pending="pending"
              :show-skeleton="showSkeleton"
              :show-empty="showEmpty"
              :show-error="showError"
              :refreshing="refreshing"
              :error="error"
              :empty-title="isAdminProject ? t('pages.cronJobs.emptyAdminTitle') : t('pages.cronJobs.emptyTitle')"
              :empty-description="isAdminProject ? t('pages.cronJobs.emptyAdminDesc') : t('pages.cronJobs.emptyDesc')"
              :empty-action-text="isAdminProject ? undefined : t('pages.cronJobs.create')"
              @retry="load"
              @empty-action="openCreate()"
            >
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
                        {{ t('pages.cronJobs.nextRun', { time: formatTime(record.nextRunAt) }) }}
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
                              {{ targetText(record) }}
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>{{ t('pages.cronJobs.targetMissing') }}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </template>
                    <template v-else>{{ targetText(record) }}</template>
                  </span>
                </TableCell>
                <TableCell>
                  <div class="flex flex-col gap-0.5">
                    <Badge :variant="cronStatusVariant(record.lastStatus)" class="w-fit">
                      {{ cronStatusText(record.lastStatus) }}
                    </Badge>
                    <span class="text-xs text-muted-foreground">
                      {{ record.lastRunAt ? formatTime(record.lastRunAt) : t('pages.cronJobs.neverRun') }}
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
                  <div class="flex flex-wrap justify-center gap-1">
                    <Button variant="ghost" size="sm" @click="openRuns(record, { trigger: true })">
                      {{ t('pages.cronJobs.runNow') }}
                    </Button>
                    <Button variant="ghost" size="sm" @click="openRuns(record)">
                      {{ t('pages.cronJobs.runs') }}
                    </Button>
                    <Button v-if="!isAdminProject" variant="ghost" size="sm" @click="openEdit(record)">
                      {{ t('pages.cronJobs.edit') }}
                    </Button>
                    <ConfirmAction
                      v-if="!isAdminProject"
                      :title="t('pages.cronJobs.deleteConfirm', { name: record.name })"
                      @confirm="remove(record)"
                    >
                      <Button variant="destructiveGhost" size="sm">
                        {{ t('pages.cronJobs.delete') }}
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

      <CronJobModal
        v-model:open="modalOpen"
        :target="modalTarget"
        :preset-agent-id="modalPresetAgentId"
        @saved="load"
      />
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
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '../services/api'
import type { CronJobItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useLoadState } from '../composables/useLoadState'
import { usePagination } from '../composables/usePagination'
import { cronStatusText, cronStatusVariant } from '@/lib/status'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import CronJobModal from '../components/modal/CronJobModal.vue'
import CronJobRunsModal from '../components/modal/CronJobRunsModal.vue'

const projectStore = useProjectStore()
const { t } = useI18n()
const { projectId, isAdmin: isAdminProject } = storeToRefs(projectStore)

const records = ref<CronJobItem[]>([])
const toggling = ref(new Set<string>())
const triggering = ref(new Set<string>())

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.cronJobs.loadFailed'
})

/** 助手 id → 名称，仅用于列表展示；加载失败时回退显示 id，不影响主列表。 */
const agentNames = ref(new Map<string, string>())

async function loadAgentNames() {
  try {
    const list = await api.agents.list(projectId.value)
    agentNames.value = new Map(list.map((a) => [a.id, a.name]))
  } catch {
    agentNames.value = new Map()
  }
}

function load() {
  void loadAgentNames()
  return run(async () => {
    const data = await api.cronjobs.list(projectId.value)
    records.value = data
    page.value = 1
    return data.length > 0
  })
}

/** 目标列文案：云函数 file.export；助手「助手：名称」。 */
function targetText(record: CronJobItem): string {
  if (record.targetKind === 'agent') {
    const id = record.agentId ?? ''
    return t('pages.cronJobs.targetAgent', { name: agentNames.value.get(id) || id })
  }
  return `${record.funcFile}.${record.funcExport}`
}

/** 调度人类化：cron 原样等宽；interval 转「每 N 分钟/小时/天」；once 显示执行时刻 */
function scheduleText(record: CronJobItem): string {
  if (record.scheduleKind === 'cron') return record.cronExpr
  if (record.scheduleKind === 'once') return record.runAt ? formatTime(record.runAt) : '—'
  const s = record.intervalSeconds ?? 0
  if (s % 86400 === 0) return t('pages.cronJobs.everyDays', { n: s / 86400 })
  if (s % 3600 === 0) return t('pages.cronJobs.everyHours', { n: s / 3600 })
  if (s % 60 === 0) return t('pages.cronJobs.everyMinutes', { n: s / 60 })
  return t('pages.cronJobs.everySeconds', { n: s })
}

function scheduleKindText(kind: CronJobItem['scheduleKind']): string {
  if (kind === 'cron') return t('cron.kindCron')
  if (kind === 'once') return t('cron.kindOnce')
  return t('cron.kindInterval')
}

async function toggleEnabled(record: CronJobItem) {
  toggling.value.add(record.id)
  try {
    const updated = await api.cronjobs.update(projectId.value, record.id, { enabled: !record.enabled })
    Object.assign(record, updated)
    toast.success(
      updated.enabled
        ? t('pages.cronJobs.enabledToast', { name: record.name })
        : t('pages.cronJobs.pausedToast', { name: record.name })
    )
  } catch (e) {
    toast.error(errorMessage(e, t('pages.cronJobs.toggleFailed')))
  } finally {
    toggling.value.delete(record.id)
  }
}

async function trigger(record: CronJobItem) {
  triggering.value.add(record.id)
  try {
    await api.cronjobs.trigger(projectId.value, record.id)
    toast.success(t('pages.cronJobs.triggeredToast', { name: record.name }))
    openRuns(record)
  } catch (e) {
    toast.error(errorMessage(e, t('pages.cronJobs.triggerFailed')))
  } finally {
    triggering.value.delete(record.id)
  }
}

async function remove(record: CronJobItem) {
  try {
    await api.cronjobs.remove(projectId.value, record.id)
    toast.success(t('pages.cronJobs.deletedToast', { name: record.name }))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.cronJobs.deleteFailed')))
  }
}

/* ---------- 弹窗与抽屉 ---------- */

const modalOpen = ref(false)
const modalTarget = ref<CronJobItem>()
const modalPresetAgentId = ref<string>()

function openCreate(presetAgentId?: string) {
  modalTarget.value = undefined
  modalPresetAgentId.value = presetAgentId
  modalOpen.value = true
}
function openEdit(record: CronJobItem) {
  modalTarget.value = record
  modalPresetAgentId.value = undefined
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

void load()
watch(projectId, () => {
  void load()
})

/** AgentManager 的「定时任务」菜单跳转而来：?agent=<id> 直接打开新建弹窗并预选该助手。 */
const route = useRoute()
const router = useRouter()
const presetFromQuery = typeof route.query.agent === 'string' ? route.query.agent : ''
if (presetFromQuery) {
  openCreate(presetFromQuery)
  void router.replace({ name: route.name ?? undefined, query: {} })
}
</script>
