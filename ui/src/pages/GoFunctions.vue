<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.gofunctions.subtitle')">
      <Card>
        <CardHeader class="flex-wrap gap-3 border-b max-sm:[&_[data-slot=card-action]]:w-full max-sm:[&_[data-slot=card-action]]:justify-start">
          <CardTitle>{{ t('pages.gofunctions.cardTitle') }}</CardTitle>
          <CardDescription class="min-w-0 break-words">
            {{ t('pages.gofunctions.cardDesc', { n: total }) }}
            <code class="sb-code-view sb-mono max-w-full min-w-0 whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] text-xs">POST /go/{{ projectId }}/{name}/{FunctionName}</code>
          </CardDescription>
          <CardAction v-if="!isAdminProject">
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="pending" @click="load">
                <Spinner v-if="pending" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                {{ t('common.refresh') }}
              </Button>
              <Button size="sm" @click="openCreate">
                <PlusIcon data-icon="inline-start" />
                {{ t('pages.gofunctions.create') }}
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
          <p class="px-4 pt-3 text-xs text-muted-foreground md:hidden">{{ t('pages.gofunctions.scrollHint') }}</p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="min-w-40 max-w-52">{{ t('pages.gofunctions.colFile') }}</TableHead>
                <TableHead class="max-w-md">{{ t('pages.gofunctions.colExports') }}</TableHead>
                <TableHead class="w-18 max-w-18">{{ t('pages.gofunctions.colActiveVersion') }}</TableHead>
                <TableHead class="w-24 max-w-28">{{ t('pages.gofunctions.colLatestVersion') }}</TableHead>
                <TableHead class="sb-col-md">{{ t('pages.gofunctions.colUpdatedAt') }}</TableHead>
                <TableHead class="min-w-72">{{ t('pages.gofunctions.colActions') }}</TableHead>
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
              :empty-title="isAdminProject ? t('pages.gofunctions.emptyAdminTitle') : t('pages.gofunctions.emptyTitle')"
              :empty-description="isAdminProject ? t('pages.gofunctions.emptyAdminDesc') : t('pages.gofunctions.emptyDesc')"
              :empty-action-text="isAdminProject ? undefined : t('pages.gofunctions.create')"
              @retry="load"
              @empty-action="openCreate"
            >
              <TableRow v-for="record in paged" :key="record.id">
                <TableCell class="max-w-52">
                  <span class="sb-mono inline-flex items-center gap-2 font-medium">
                    <CodeIcon class="size-4 shrink-0 text-primary" />
                    <span class="truncate">{{ record.file }}</span>
                  </span>
                </TableCell>
                <TableCell>
                  <div class="flex max-w-md flex-wrap gap-1.5">
                    <Badge
                      v-for="fn in record.exports"
                      :key="fn"
                      as="button"
                      type="button"
                      variant="secondary"
                      :aria-label="t('pages.gofunctions.copyPathAria', { file: record.file, fn })"
                      class="sb-code-view sb-mono h-auto min-h-8 max-w-full min-w-0 cursor-pointer whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] text-left hover:bg-secondary/70"
                      @click="copyInvokePath(record, fn)"
                    >
                      {{ fn }}
                    </Badge>
                    <span v-if="!record.exports.length" class="text-sm text-muted-foreground">—</span>
                  </div>
                </TableCell>
                <TableCell class="w-18 max-w-18">
                  <Badge v-if="record.activeVersion > 0" variant="success" class="px-1.5">
                    v{{ record.activeVersion }}
                  </Badge>
                  <Badge v-else variant="outline" class="px-1.5">{{ t('pages.gofunctions.unpublished') }}</Badge>
                </TableCell>
                <TableCell class="w-24 max-w-28 tabular-nums">
                  <div class="flex flex-col gap-0.5">
                    <span>v{{ record.latestVersion || '—' }}</span>
                    <span
                      v-if="record.latestVersion > record.activeVersion && record.activeVersion > 0"
                      class="text-xs text-warning"
                    >{{ t('pages.gofunctions.hasUnpublished') }}</span>
                  </div>
                </TableCell>
                <TableCell class="sb-col-md text-xs text-muted-foreground">
                  {{ formatTime(record.updatedAt) }}
                </TableCell>
                <TableCell class="min-w-72">
                  <div class="flex flex-nowrap items-center justify-center gap-1">
                    <Button v-if="!isAdminProject" variant="outline" size="sm" class="px-2" @click="openTest(record)">
                      {{ t('pages.gofunctions.test') }}
                    </Button>
                    <Button variant="ghost" size="sm" class="px-2" @click="openView(record)">{{ t('pages.gofunctions.view') }}</Button>
                    <Button variant="ghost" size="sm" class="px-2" @click="openVersions(record)">{{ t('pages.gofunctions.versions') }}</Button>
                    <Button v-if="!isAdminProject" variant="ghost" size="sm" class="px-2" @click="openEdit(record)">{{ t('common.edit') }}</Button>
                    <ConfirmAction
                      v-if="!isAdminProject"
                      :title="t('pages.gofunctions.deleteConfirm', { file: record.file })"
                      @confirm="remove(record)"
                    >
                      <Button variant="destructiveGhost" size="sm" class="px-2">{{ t('common.del') }}</Button>
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

      <GoFunctionModal v-model:open="modalOpen" :mode="modalMode" :target="modalTarget" @saved="load" />
      <GoFuncTestModal v-model:open="testOpen" :record="testTarget" />
      <GoFuncVersionsModal
        v-model:open="versionsOpen"
        :record="versionsTarget"
        @changed="load"
      />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
/**
 * 云函数列表页（ui-gofunction-plan §8.3）。
 * Badge 点击复制完整调用 URL；删除走 ConfirmAction；项目切换重载。
 */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { api } from '../services/api'
import type { GoFunctionItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useLoadState } from '../composables/useLoadState'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import TablePager from '../components/TablePager.vue'
import GoFunctionModal from '../components/modal/GoFunctionModal.vue'
import GoFuncTestModal from '../components/modal/GoFuncTestModal.vue'
import GoFuncVersionsModal from '../components/modal/GoFuncVersionsModal.vue'

const projectStore = useProjectStore()
const { projectId, isAdmin: isAdminProject } = storeToRefs(projectStore)
const { t } = useI18n()

const records = ref<GoFunctionItem[]>([])

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.gofunctions.loadFailed'
})

function load() {
  return run(async () => {
    const data = await api.gofunctions.list(projectId.value)
    records.value = data
    page.value = 1
    return data.length > 0
  })
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

/* 测试台 / 版本管理 */
const testOpen = ref(false)
const testTarget = ref<GoFunctionItem | null>(null)
const versionsOpen = ref(false)
const versionsTarget = ref<GoFunctionItem | null>(null)

function openTest(record: GoFunctionItem) {
  testTarget.value = record
  testOpen.value = true
}
function openVersions(record: GoFunctionItem) {
  versionsTarget.value = record
  versionsOpen.value = true
}

async function remove(record: GoFunctionItem) {
  try {
    await api.gofunctions.remove(projectId.value, record.name)
    toast.success(t('pages.gofunctions.deleteSuccess', { file: record.file }))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.gofunctions.deleteFailed')))
  }
}

function copyInvokePath(record: GoFunctionItem, fn?: string) {
  if (!fn) {
    toast.error(t('pages.gofunctions.noExports'))
    return
  }
  const path = `POST /go/${projectId.value}/${record.name}/${fn}`
  navigator.clipboard
    .writeText(path)
    .then(() => toast.success(t('pages.gofunctions.copiedPath', { path })))
    .catch(() => toast.error(t('pages.gofunctions.copyFailed')))
}

void load()
watch(projectId, () => {
  void load()
})
</script>
