<template>
  <Card>
    <CardHeader class="flex-wrap gap-3 border-b max-sm:[&_[data-slot=card-action]]:w-full max-sm:[&_[data-slot=card-action]]:justify-start">
      <CardTitle>{{ t('pages.approvals.cardTitle') }}</CardTitle>
      <CardDescription>{{ t('pages.approvals.cardDesc', { n: total, pending }) }}</CardDescription>
      <CardAction>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" :disabled="loading || isAdmin" @click="load">
            <Spinner v-if="loading" data-icon="inline-start" />
            <RefreshCwIcon v-else data-icon="inline-start" />
            {{ t('common.refresh') }}
          </Button>
        </div>
      </CardAction>
    </CardHeader>

    <div class="flex flex-wrap gap-2 border-b p-3 text-sm">
      <div class="flex items-center gap-2">
        <span>{{ t('pages.approvals.filterStatus') }}</span>
        <Select :model-value="statusFilter" @update:model-value="(v) => (statusFilter = String(v))">
          <SelectTrigger class="h-8 w-36" :aria-label="t('pages.approvals.filterStatusAria')">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem :value="FILTER_ALL">{{ t('common.all') }}</SelectItem>
            <SelectItem v-for="s in APPROVAL_STATUSES" :key="s" :value="s">
              {{ t(`pages.approvals.status.${s}`) }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>

    <div class="p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="sb-col-sm">{{ t('pages.approvals.colStatus') }}</TableHead>
            <TableHead class="min-w-56">{{ t('pages.approvals.colCommand') }}</TableHead>
            <TableHead class="sb-col-name">{{ t('pages.approvals.colMessage') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('pages.approvals.colInitiator') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('pages.approvals.colCreated') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('pages.approvals.colExpires') }}</TableHead>
            <TableHead class="sb-col-act min-w-40">{{ t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="7"
          :pending="loading"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-title="isAdmin ? t('pages.approvals.emptyAdminTitle') : t('pages.approvals.emptyTitle')"
          :empty-description="isAdmin ? t('pages.approvals.emptyAdminDesc') : t('pages.approvals.emptyDesc')"
          @retry="load"
        >
          <TableRow v-for="item in paged" :key="item.id">
            <TableCell>
              <Badge :variant="statusVariant(item.status)">{{ t(`pages.approvals.status.${item.status}`) }}</Badge>
            </TableCell>
            <TableCell>
              <div class="flex max-w-96 flex-col gap-0.5">
                <span class="sb-mono truncate text-xs" :title="item.tool_name">{{ item.tool_name || '—' }}</span>
                <span
                  v-if="item.argv.length"
                  class="sb-mono truncate text-xs text-muted-foreground"
                  :title="item.argv.join(' ')"
                >{{ item.argv.join(' ') }}</span>
              </div>
            </TableCell>
            <TableCell>
              <span class="line-clamp-2 max-w-72 text-xs text-muted-foreground" :title="item.message">
                {{ item.message || '—' }}
              </span>
            </TableCell>
            <TableCell><span class="sb-mono text-xs">{{ item.initiator_id || '—' }}</span></TableCell>
            <TableCell class="text-xs">{{ formatTime(item.created_at) }}</TableCell>
            <TableCell class="text-xs">{{ item.expires_at ? formatTime(item.expires_at) : '—' }}</TableCell>
            <TableCell>
              <div v-if="item.status === 'pending' && auth.canWrite" class="flex flex-wrap justify-center gap-1">
                <Button size="sm" :disabled="resolving.has(item.id)" @click="resolve(item, true)">
                  <Spinner v-if="resolving.has(item.id)" data-icon="inline-start" />
                  {{ t('pages.approvals.approve') }}
                </Button>
                <Button
                  size="sm"
                  variant="destructiveGhost"
                  :disabled="resolving.has(item.id)"
                  @click="resolve(item, false)"
                >
                  {{ t('pages.approvals.deny') }}
                </Button>
              </div>
              <span v-else-if="item.resolved_by" class="sb-mono text-xs text-muted-foreground">
                {{ item.resolved_by }}
              </span>
              <span v-else class="text-xs text-muted-foreground">—</span>
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
</template>

<script setup lang="ts">
/**
 * 审批队列面板（原 /console/approvals 页，planv6.6）。
 *
 * 挂在「模型」页的审批队列页签：审批与模型/MCP 同属「助手运行时配置」，
 * 拆成独立侧栏项会让工作台一级菜单过密。
 *
 * 与对话内的行内确认卡（AgentConfirmCard）分工：
 *  - 对话内确认只覆盖「当前连接还在等」的场景；
 *  - 本面板覆盖「离开对话后仍有待办」的场景，数据来自持久表，可跨请求查询。
 * 放行/拒绝复用 run 的确认端点，后端会同步收敛队列状态。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { RefreshCwIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { AgentApproval, AgentApprovalStatus } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { errorMessage, formatTime } from '@/utils/format'
import { useLoadState } from '@/composables/useLoadState'
import { usePagination } from '@/composables/usePagination'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import TablePager from '@/components/TablePager.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const { t } = useI18n()
const project = useProjectStore()
const auth = useAuthStore()
const { projectId, isAdmin } = storeToRefs(project)

/** 状态枚举（值本身是后端常量，不翻译）。 */
const APPROVAL_STATUSES: AgentApprovalStatus[] = ['pending', 'approved', 'denied', 'expired', 'canceled']
/** reka-ui 的 SelectItem 不允许空字符串 value，用哨兵值表示「全部」。 */
const FILTER_ALL = '__all__'

const records = ref<AgentApproval[]>([])
const pendingCount = ref(0)
const statusFilter = ref<string>(FILTER_ALL)
const resolving = ref(new Set<string>())
let listRequest = 0

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const {
  pending: loading,
  showSkeleton,
  showEmpty,
  showError,
  refreshing,
  error,
  run
} = useLoadState({ fallback: 'pages.approvals.loadFailed' })

/** 角标用 pending 总数；列表可能被状态过滤，两者不是同一个数。 */
const pending = computed(() => pendingCount.value)

function statusVariant(status: AgentApprovalStatus) {
  if (status === 'pending') return 'warning'
  if (status === 'approved') return 'success'
  if (status === 'denied') return 'destructive'
  return 'secondary'
}

function load() {
  const currentProject = projectId.value
  const request = ++listRequest
  return run(async () => {
    if (!currentProject || isAdmin.value) {
      records.value = []
      pendingCount.value = 0
      page.value = 1
      return false
    }
    const status = statusFilter.value === FILTER_ALL ? '' : statusFilter.value
    const data = await api.agentApprovals.list(currentProject, status)
    if (request !== listRequest || currentProject !== projectId.value) return records.value.length > 0
    records.value = data.approvals || []
    pendingCount.value = data.pending || 0
    return records.value.length > 0
  })
}

async function resolve(item: AgentApproval, approve: boolean) {
  if (!auth.canWrite || resolving.value.has(item.id)) return
  resolving.value.add(item.id)
  try {
    await api.agentApprovals.resolve(projectId.value, item.run_id, item.call_id, approve)
    toast.success(t(approve ? 'pages.approvals.approvedToast' : 'pages.approvals.deniedToast'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.approvals.resolveFailed')))
  } finally {
    resolving.value.delete(item.id)
  }
}

watch(statusFilter, () => {
  page.value = 1
  void load()
})
watch(
  projectId,
  () => {
    statusFilter.value = FILTER_ALL
    page.value = 1
    void load()
  },
  { immediate: true }
)
</script>