<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.sandboxes.subtitle')">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>{{ t('pages.sandboxes.cardTitle') }}</CardTitle>
          <CardDescription>{{ t('pages.sandboxes.cardDesc', { n: records.length }) }}</CardDescription>
          <CardAction>
            <div class="flex gap-2">
              <Button variant="outline" size="sm" :disabled="pending" @click="load">
                <Spinner v-if="pending" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                {{ t('common.refresh') }}
              </Button>
              <Button v-if="capabilities?.available && !isAdminProject && auth.canWrite" size="sm" @click="createOpen = true">
                <PlusIcon data-icon="inline-start" />
                {{ t('pages.sandboxes.create') }}
              </Button>
            </div>
          </CardAction>
        </CardHeader>
        <div v-if="notice" class="p-6">
          <SbAsyncRegion :show-empty="true" :pending="false" :show-skeleton="false" :show-error="false">
            <template #empty>
              <template v-if="notice === 'unconfigured'">
                <SbEmptyState :title="t('pages.sandboxes.notConfiguredTitle')" :description="t('pages.sandboxes.notConfiguredDesc')" />
                <pre class="overflow-auto rounded-md bg-muted p-3 text-xs">sandbox:\n  enabled: true\n  backend: local</pre>
              </template>
              <SbEmptyState v-else :title="t('pages.sandboxes.adminUnsupported')" :description="t('pages.sandboxes.adminUnsupportedDesc')" />
            </template>
          </SbAsyncRegion>
        </div>
        <div v-else class="p-0">
          <div class="flex flex-wrap gap-2 border-b p-3 text-sm">
            <div class="flex items-center gap-2">
              <span>{{ t('pages.sandboxes.filterStatus') }}</span>
              <Select :model-value="statusFilter" @update:model-value="(v) => (statusFilter = String(v))">
                <SelectTrigger class="h-8 w-32" :aria-label="t('pages.sandboxes.filterStatusAria')">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem :value="FILTER_ALL">{{ t('common.all') }}</SelectItem>
                  <SelectItem v-for="s in SANDBOX_STATUSES" :key="s" :value="s">{{ s }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="flex items-center gap-2">
              <span>{{ t('pages.sandboxes.filterSource') }}</span>
              <Select :model-value="sourceFilter" @update:model-value="(v) => (sourceFilter = String(v))">
                <SelectTrigger class="h-8 w-32" :aria-label="t('pages.sandboxes.filterSourceAria')">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem :value="FILTER_ALL">{{ t('common.all') }}</SelectItem>
                  <SelectItem value="api">{{ t('pages.sandboxes.sourceApi') }}</SelectItem>
                  <SelectItem value="console">{{ t('pages.sandboxes.sourceConsole') }}</SelectItem>
                  <SelectItem value="agent">{{ t('pages.sandboxes.sourceAgent') }}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ t('pages.sandboxes.colName') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colStatus') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colTemplate') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colSpec') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colSource') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colLastActive') }}</TableHead>
                <TableHead>{{ t('pages.sandboxes.colExpires') }}</TableHead>
                <TableHead>{{ t('common.actions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <SbAsyncRegion
              as="tbody"
              :columns="8"
              :pending="pending"
              :show-skeleton="showSkeleton"
              :show-empty="showEmpty"
              :show-error="showError"
              :refreshing="refreshing"
              :error="error"
              :empty-title="t('pages.sandboxes.emptyTitle')"
              :empty-description="t('pages.sandboxes.emptyDesc')"
              @retry="load"
            >
              <TableRow v-for="item in filtered" :key="item.id">
                <TableCell class="sb-mono">{{ item.name }}</TableCell>
                <TableCell>
                  <Badge :variant="item.status === 'error' ? 'destructive' : item.state === 'running' ? 'default' : 'secondary'">{{ item.state }}</Badge>
                </TableCell>
                <TableCell class="sb-mono">{{ item.templateId }}</TableCell>
                <TableCell>{{ item.cpuCount }}C / {{ item.memoryMB }}M</TableCell>
                <TableCell>{{ sourceText(item.source) }}</TableCell>
                <TableCell>{{ item.lastActiveAt ? formatTime(item.lastActiveAt) : t('common.notStarted') }}</TableCell>
                <TableCell>{{ item.endAt ? formatTime(item.endAt) : '—' }}</TableCell>
                <TableCell>
                  <div class="flex flex-wrap justify-center gap-1">
                    <Button variant="ghost" size="sm" @click="selected = item">{{ t('pages.sandboxes.open') }}</Button>
                    <Button
                      v-if="auth.canWrite && item.state === 'running'"
                      variant="ghost"
                      size="sm"
                      :disabled="stopping.has(item.id)"
                      @click="pause(item)"
                    >
                      <Spinner v-if="stopping.has(item.id)" data-icon="inline-start" />
                      {{ t('pages.sandboxes.stop') }}
                    </Button>
                    <Button
                      v-if="auth.canWrite && item.state === 'paused'"
                      variant="ghost"
                      size="sm"
                      :disabled="starting.has(item.id)"
                      @click="resume(item)"
                    >
                      <Spinner v-if="starting.has(item.id)" data-icon="inline-start" />
                      {{ t('pages.sandboxes.start') }}
                    </Button>
                    <ConfirmAction
                      v-if="auth.canWrite"
                      :title="t('pages.sandboxes.deleteConfirm', { name: item.name })"
                      :disabled="deleting.has(item.id)"
                      @confirm="remove(item)"
                    >
                      <Button variant="destructiveGhost" size="sm" :disabled="deleting.has(item.id)">
                        <Spinner v-if="deleting.has(item.id)" data-icon="inline-start" />
                        {{ t('common.del') }}
                      </Button>
                    </ConfirmAction>
                  </div>
                </TableCell>
              </TableRow>
              <TableEmpty v-if="filtered.length === 0" :colspan="8">
                <SbEmptyState :title="t('pages.sandboxes.emptyTitle')" :description="t('pages.sandboxes.emptyDesc')" />
              </TableEmpty>
            </SbAsyncRegion>
          </Table>
        </div>
      </Card>
      <SandboxCreateModal v-model:open="createOpen" :project-id="projectId" :capabilities="capabilities" @saved="load" />
      <SandboxDrawer :open="!!selected" :project-id="projectId" :sandbox="selected" :capabilities="capabilities" @update:open="!$event && (selected = null)" @changed="load" />
    </PageContainer>
  </ProjectScope>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableHeader, TableRow, TableHead, TableCell, TableEmpty } from '@/components/ui/table'
import { api } from '../services/api'
import type { SandboxCapabilities, SandboxItem } from '../services/types'
import { useProjectStore } from '../stores/project'
import { useAuthStore } from '../stores/auth'
import { useLoadState } from '@/composables/useLoadState'
import { formatTime } from '../utils/format'
import PageContainer from '../components/PageContainer.vue'
import ProjectScope from '../components/ProjectScope.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import ConfirmAction from '../components/ConfirmAction.vue'
import SandboxCreateModal from '../components/modal/SandboxCreateModal.vue'
import SandboxDrawer from '../components/sandbox/SandboxDrawer.vue'

const { t } = useI18n()
const store = useProjectStore()
const auth = useAuthStore()

/** 状态过滤下拉的可选值（值本身是后端枚举，不翻译）。 */
const SANDBOX_STATUSES = ['running', 'paused']
const { projectId, isAdmin: isAdminProject } = storeToRefs(store)
const capabilities = ref<SandboxCapabilities | null>(null)
const records = ref<SandboxItem[]>([])
/** reka-ui 的 SelectItem 不允许空字符串 value，用哨兵值表示「全部」。 */
const FILTER_ALL = '__all__'
const statusFilter = ref(FILTER_ALL)
const sourceFilter = ref(FILTER_ALL)
const createOpen = ref(false)
const selected = ref<SandboxItem | null>(null)
const starting = ref(new Set<string>())
const stopping = ref(new Set<string>())
const deleting = ref(new Set<string>())
const { pending, settled, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.sandboxes.loadFailed'
})

/** 来源枚举 → 展示文案。 */
function sourceText(source: string): string {
  if (source === 'agent') return t('pages.sandboxes.sourceAgent')
  if (source === 'api') return t('pages.sandboxes.sourceApi')
  return t('pages.sandboxes.sourceConsole')
}

const filtered = computed(() => records.value.filter((s) =>
  (statusFilter.value === FILTER_ALL || s.state === statusFilter.value) &&
  (sourceFilter.value === FILTER_ALL || s.source === sourceFilter.value)))

const notice = computed(() => {
  if (!settled.value || pending.value || showError.value) return ''
  if (capabilities.value && !capabilities.value.available) return 'unconfigured'
  if (isAdminProject.value) return 'admin'
  return ''
})

function load() {
  if (!projectId.value) return
  const id = projectId.value
  return run(async () => {
    const caps = await api.sandboxes.capabilities(id)
    const listed = caps.available ? await api.sandboxes.list(id) : []
    if (projectId.value !== id) return false
    capabilities.value = caps
    records.value = listed
    if (selected.value) selected.value = listed.find((item) => item.id === selected.value?.id) || null
    if (!caps.available || isAdminProject.value) return false
    return listed.length > 0
  })
}

async function runRow(bucket: typeof starting, id: string, task: () => Promise<void>) {
  if (bucket.value.has(id)) return
  bucket.value.add(id)
  try {
    await task()
  } finally {
    bucket.value.delete(id)
  }
}

async function resume(item: SandboxItem) {
  await runRow(starting, item.id, async () => {
    try {
      await api.sandboxes.connect(projectId.value, item.id)
      await load()
      toast.success(t('pages.sandboxes.startedToast'))
    } catch (e) {
      toast.error(errorMessage(e, t('pages.sandboxes.startFailed')))
    }
  })
}

async function pause(item: SandboxItem) {
  await runRow(stopping, item.id, async () => {
    try {
      await api.sandboxes.pause(projectId.value, item.id)
      await load()
      toast.success(t('pages.sandboxes.stoppedToast'))
    } catch (e) {
      toast.error(errorMessage(e, t('pages.sandboxes.stopFailed')))
    }
  })
}

async function remove(item: SandboxItem) {
  await runRow(deleting, item.id, async () => {
    try {
      await api.sandboxes.remove(projectId.value, item.id)
      await load()
      toast.success(t('pages.sandboxes.deletedToast'))
    } catch (e) {
      toast.error(errorMessage(e, t('pages.sandboxes.deleteFailed')))
    }
  })
}

void load()
watch(projectId, () => {
  void load()
})
</script>
