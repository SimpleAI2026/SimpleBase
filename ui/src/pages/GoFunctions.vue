<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.gofunctions.subtitle')">
      <SbListCard :title="t('pages.gofunctions.cardTitle')" :pager="{ page, pageSize, total, pageCount }" @update:page="page = $event">
        <template #description>
          {{ t('pages.gofunctions.cardDesc', { n: total }) }}
          <code class="sb-code-view sb-mono max-w-full min-w-0 whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere] text-xs">POST /go/{{ projectId }}/{name}/{FunctionName}</code>
        </template>
        <template #actions>
          <SbRefreshButton :pending="pending" @click="load" />
          <Button v-if="canWrite" size="sm" @click="openCreate">
            <PlusIcon data-icon="inline-start" />
            {{ t('pages.gofunctions.create') }}
          </Button>
        </template>
        <div>
          <p class="px-4 pt-3 text-xs text-muted-foreground md:hidden">{{ t('pages.gofunctions.scrollHint') }}</p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="min-w-40 max-w-52">{{ t('pages.gofunctions.colFile') }}</TableHead>
                <TableHead class="max-w-md">{{ t('pages.gofunctions.colExports') }}</TableHead>
                <TableHead class="w-18 max-w-18">{{ t('pages.gofunctions.colActiveVersion') }}</TableHead>
                <TableHead class="w-24 max-w-28">{{ t('pages.gofunctions.colLatestVersion') }}</TableHead>
                <TableHead class="sb-col-md">{{ t('pages.gofunctions.colUpdatedAt') }}</TableHead>
                <TableHead class="sb-col-act min-w-40">{{ t('pages.gofunctions.colActions') }}</TableHead>
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
              :empty-action-text="canWrite ? t('pages.gofunctions.create') : undefined"
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
                <TableCell class="sb-col-act min-w-40">
                  <div class="flex flex-wrap items-center justify-center gap-1">
                    <Button v-if="canWrite" variant="ghost" size="sm" @click="openTest(record)">
                      {{ t('pages.gofunctions.test') }}
                    </Button>
                    <Button v-if="canWrite" variant="ghost" size="sm" @click="openEdit(record)">{{ t('common.edit') }}</Button>
                    <Button variant="ghost" size="sm" @click="openVersions(record)">{{ t('pages.gofunctions.versions') }}</Button>
                    <ConfirmAction
                      v-if="canWrite"
                      :title="t('pages.gofunctions.deleteConfirm', { file: record.file })"
                      @confirm="remove(record)"
                    >
                      <Button variant="destructiveGhost" size="sm">{{ t('common.del') }}</Button>
                    </ConfirmAction>
                  </div>
                </TableCell>
              </TableRow>
          </SbAsyncRegion>
          </Table>
        </div>
      </SbListCard>

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
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { CodeIcon, PlusIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { api } from '../services/api'
import type { GoFunctionItem } from '../services/api'
import { useProjectStore } from '../stores/project'
import { useAuthStore } from '../stores/auth'
import { useLoadState } from '../composables/useLoadState'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import SbRefreshButton from '../components/SbRefreshButton.vue'
import SbListCard from '../components/SbListCard.vue'
import { usePagination } from '../composables/usePagination'
import { formatTime } from '../utils/format'
import { copyText } from '../utils/clipboard'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import GoFunctionModal from '../components/modal/GoFunctionModal.vue'
import GoFuncTestModal from '../components/modal/GoFuncTestModal.vue'
import GoFuncVersionsModal from '../components/modal/GoFuncVersionsModal.vue'

const projectStore = useProjectStore()
const { projectId, isAdmin: isAdminProject } = storeToRefs(projectStore)
const auth = useAuthStore()
const { t } = useI18n()
/** 写入口：系统项目与只读角色都隐藏（planv6.7 ui-simplify-ux-plan B11）。 */
const canWrite = computed(() => !isAdminProject.value && auth.canWrite)

const records = ref<GoFunctionItem[]>([])

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.gofunctions.loadFailed'
})

function load() {
  if (!projectId.value) return Promise.resolve()
  return run(async () => {
    const data = await api.gofunctions.list(projectId.value)
    records.value = data
    // 项目切换才回到第 1 页；保存 / 删除后刷新保留当前页（planv6.7 B2）
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
  const url = `${window.location.origin}/go/${encodeURIComponent(projectId.value)}/${encodeURIComponent(record.name)}/${encodeURIComponent(fn)}`
  void copyText(url, t('pages.gofunctions.copiedPath', { path: url }), t('pages.gofunctions.copyFailed'))
}

void load()
watch(projectId, () => {
  page.value = 1
  void load()
})
</script>
