<template>
  <Card>
    <CardHeader class="border-b">
      <CardTitle>{{ t('pages.traces.title') }}</CardTitle>
      <CardDescription>{{ t('pages.traces.subtitle') }}</CardDescription>
    </CardHeader>
    <CardContent class="flex flex-wrap items-end gap-2.5 py-4">
      <div class="flex flex-col gap-1">
        <span class="text-xs text-muted-foreground">{{ t('pages.traces.status') }}</span>
        <Select :model-value="statusFilter" @update:model-value="onStatusChange">
          <SelectTrigger class="w-36" size="sm">
            <SelectValue :placeholder="t('pages.traces.statusAll')" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="ok">{{ t('pages.traces.statusOk') }}</SelectItem>
              <SelectItem value="error">{{ t('pages.traces.statusError') }}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <div class="flex flex-col gap-1">
        <span class="text-xs text-muted-foreground">{{ t('pages.traces.filterCaller') }}</span>
        <Select :model-value="callerFilter" @update:model-value="onCallerChange">
          <SelectTrigger class="w-36" size="sm">
            <SelectValue :placeholder="t('pages.traces.callerAll')" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="api">{{ t('pages.traces.callerApi') }}</SelectItem>
              <SelectItem value="agent">{{ t('pages.traces.callerAgent') }}</SelectItem>
              <SelectItem value="cron">{{ t('pages.traces.callerCron') }}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <div class="flex min-w-55 flex-1 flex-col gap-1 sm:max-w-80">
        <span class="text-xs text-muted-foreground">{{ t('pages.traces.filterModel') }}</span>
        <InputGroup>
          <InputGroupAddon>
            <FilterIcon />
          </InputGroupAddon>
          <InputGroupInput
            v-model="modelFilter"
            :placeholder="t('pages.traces.filterModel')"
            @keydown.enter="load"
          />
        </InputGroup>
      </div>
      <Button size="sm" :disabled="pending" @click="load">
        <Spinner v-if="pending" data-icon="inline-start" />
        <RefreshCwIcon v-else data-icon="inline-start" />
        {{ t('common.refresh') }}
      </Button>
      <span class="w-full text-xs text-muted-foreground sm:ml-auto sm:w-auto sm:pb-1">
        {{ t('pages.traces.total', { n: total }) }}
      </span>
    </CardContent>
    <div class="p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="sb-col-md">{{ t('pages.traces.time') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('pages.traces.status') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('pages.traces.caller') }}</TableHead>
            <TableHead class="sb-col-md">{{ t('pages.traces.model') }}</TableHead>
            <TableHead class="sb-col-sm text-right">{{ t('pages.traces.spans') }}</TableHead>
            <TableHead class="sb-col-sm text-right">{{ t('pages.traces.duration') }}</TableHead>
            <TableHead class="sb-col-sm text-right">{{ t('pages.traces.inputTokens') }}</TableHead>
            <TableHead class="sb-col-sm text-right">{{ t('pages.traces.outputTokens') }}</TableHead>
            <TableHead class="max-w-md text-left">{{ t('pages.traces.error') }}</TableHead>
            <TableHead class="sb-col-actions">{{ t('pages.traces.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="10"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-description="t('pages.traces.emptyDesc')"
          :empty-action-text="hasFilter ? t('pages.logs.clearFilter') : undefined"
          @retry="load"
          @empty-action="clearFilters"
        >
          <TableRow v-for="g in groups" :key="g.traceId" class="font-mono text-xs">
            <TableCell class="text-muted-foreground">{{ formatTime(g.startedAt) }}</TableCell>
            <TableCell>
              <Badge :variant="g.status === 'ok' ? 'default' : 'destructive'">
                {{ g.status === 'ok' ? t('pages.traces.statusOk') : t('pages.traces.statusError') }}
              </Badge>
            </TableCell>
            <TableCell>
              <Badge variant="outline" class="font-sans">
                {{ callerLabel(g.caller) }}
              </Badge>
            </TableCell>
            <TableCell class="max-w-48 truncate">{{ g.model }}</TableCell>
            <TableCell class="text-right text-muted-foreground">
              {{ g.llmCount }}<template v-if="g.toolCount > 0"> + {{ g.toolCount }}🛠</template>
            </TableCell>
            <TableCell class="text-right text-muted-foreground">{{ g.durationMs }} ms</TableCell>
            <TableCell class="text-right text-muted-foreground">{{ g.inputTokens }}</TableCell>
            <TableCell class="text-right text-muted-foreground">{{ g.outputTokens }}</TableCell>
            <TableCell class="max-w-xs truncate text-left font-sans text-xs text-foreground">
              {{ g.errorCode || '—' }}
            </TableCell>
            <TableCell>
              <Button variant="ghost" size="sm" class="h-7 px-2 font-sans" @click="emit('openDetail', g.traceId)">
                {{ t('pages.traces.viewDetail') }}
                <ChevronRightIcon data-icon="inline-end" />
              </Button>
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
        @update:page="onPage"
      />
    </div>
  </Card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRightIcon, FilterIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '../../services/api'
import type { LLMTraceGroup } from '../../services/api'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import TablePager from '../TablePager.vue'
import { useProjectStore } from '../../stores/project'
import { useLoadState } from '../../composables/useLoadState'
import { formatTime } from '../../utils/format'

/**
 * 模型调用 Trace 分组列表（planv6.5 二期 §9.3 / §11.2）：
 * 一次顶层调用（trace）一行，点击行操作打开右侧详情 Sheet。
 * 服务端默认窗口 24h；分页走服务端 offset。
 */
const PAGE_SIZE = 50

const emit = defineEmits<{ openDetail: [traceId: string] }>()

const { t } = useI18n()
const projectStore = useProjectStore()
const statusFilter = ref<'ok' | 'error' | undefined>()
const callerFilter = ref<'api' | 'agent' | 'cron' | undefined>()
const modelFilter = ref('')
const page = ref(1)
const groups = ref<LLMTraceGroup[]>([])
const total = ref(0)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.traces.loadFailed'
})

const hasFilter = computed(() => Boolean(statusFilter.value || callerFilter.value || modelFilter.value.trim()))
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const pageSize = PAGE_SIZE

function callerLabel(caller: string): string {
  if (caller === 'agent') return t('pages.traces.callerAgent')
  if (caller === 'cron') return t('pages.traces.callerCron')
  return t('pages.traces.callerApi')
}

function onStatusChange(v: string) {
  statusFilter.value = v === 'ok' || v === 'error' ? v : undefined
}

function onCallerChange(v: string) {
  callerFilter.value = v === 'api' || v === 'agent' || v === 'cron' ? v : undefined
}

function load() {
  if (!projectStore.id) return Promise.resolve()
  return run(async () => {
    const result = await api.llmTraces.groups(projectStore.id, {
      status: statusFilter.value,
      caller: callerFilter.value,
      model: modelFilter.value.trim() || undefined,
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE
    })
    groups.value = result.groups
    total.value = result.total
    return result.groups.length > 0
  })
}

function clearFilters() {
  statusFilter.value = undefined
  callerFilter.value = undefined
  modelFilter.value = ''
  page.value = 1
  void load()
}

function onPage(p: number) {
  page.value = p
  void load()
}

watch([statusFilter, callerFilter, () => modelFilter.value], () => {
  page.value = 1
})

watch(
  () => projectStore.id,
  () => {
    page.value = 1
    void load()
  }
)

void load()
</script>
