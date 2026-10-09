<template>
  <ProjectScope>
    <PageContainer subtitle="按需启动的隔离 Linux 环境，空闲后自动回收">
      <Card>
        <CardHeader class="border-b">
          <CardTitle>云沙盒列表</CardTitle>
          <CardDescription>项目级隔离环境 · {{ records.length }} 个沙盒</CardDescription>
          <CardAction>
            <div class="flex gap-2">
              <Button variant="outline" size="sm" :disabled="pending" @click="load">
                <Spinner v-if="pending" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                刷新
              </Button>
              <Button v-if="capabilities?.available && !isAdminProject && auth.canWrite" size="sm" @click="createOpen = true">
                <PlusIcon data-icon="inline-start" />
                新建沙盒
              </Button>
            </div>
          </CardAction>
        </CardHeader>
        <div v-if="notice" class="p-6">
          <SbAsyncRegion :show-empty="true" :pending="false" :show-skeleton="false" :show-error="false">
            <template #empty>
              <template v-if="notice === 'unconfigured'">
                <SbEmptyState title="未配置云沙盒" description="启用 sandbox.enabled，并通过 SIMPLEBASE_SANDBOX_API_KEY 配置 Cloud 密钥。" />
                <pre class="overflow-auto rounded-md bg-muted p-3 text-xs">sandbox:\n  enabled: true\n  backend: cloud\n# SIMPLEBASE_SANDBOX_API_KEY=...</pre>
              </template>
              <SbEmptyState v-else title="系统项目不支持云沙盒" description="请切换到普通项目使用" />
            </template>
          </SbAsyncRegion>
        </div>
        <div v-else class="p-0">
          <div class="flex flex-wrap gap-2 border-b p-3 text-sm">
            <label>状态 <select v-model="statusFilter" aria-label="按状态过滤" class="rounded-md border bg-background px-2 py-1"><option value="">全部</option><option v-for="s in ['pending','running','stopped','expired','error']" :key="s" :value="s">{{ s }}</option></select></label>
            <label>来源 <select v-model="sourceFilter" aria-label="按来源过滤" class="rounded-md border bg-background px-2 py-1"><option value="">全部</option><option value="api">API</option><option value="console">控制台</option><option value="agent">Agent</option><option value="run">一次性</option></select></label>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>镜像</TableHead>
                <TableHead>规格</TableHead>
                <TableHead>来源</TableHead>
                <TableHead>最近活跃</TableHead>
                <TableHead>到期</TableHead>
                <TableHead>操作</TableHead>
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
              empty-title="还没有云沙盒"
              empty-description="新建后按需启动；空闲后会自动回收"
              @retry="load"
            >
              <TableRow v-for="item in filtered" :key="item.id">
                <TableCell class="sb-mono">{{ item.name }}</TableCell>
                <TableCell>
                  <Badge :variant="item.status === 'running' ? 'default' : item.status === 'error' ? 'destructive' : 'secondary'">{{ item.status }}</Badge>
                </TableCell>
                <TableCell class="sb-mono">{{ item.image }}</TableCell>
                <TableCell>{{ item.cpus }}C / {{ item.memoryMiB }}M</TableCell>
                <TableCell>{{ item.source === 'agent' ? 'Agent' : item.source === 'api' ? 'API' : item.source === 'run' ? '一次性' : '控制台' }}</TableCell>
                <TableCell>{{ item.lastActiveAt ? formatTime(item.lastActiveAt) : '未启动' }}</TableCell>
                <TableCell>{{ item.expiresAt ? formatTime(item.expiresAt) : '—' }}</TableCell>
                <TableCell>
                  <div class="flex flex-wrap justify-center gap-1">
                    <Button variant="ghost" size="sm" @click="selected = item">打开</Button>
                    <Button
                      v-if="auth.canWrite && item.status === 'running'"
                      variant="ghost"
                      size="sm"
                      :disabled="stopping.has(item.id)"
                      @click="stop(item)"
                    >
                      <Spinner v-if="stopping.has(item.id)" data-icon="inline-start" />
                      停止
                    </Button>
                    <Button
                      v-if="auth.canWrite && ['stopped','expired','error'].includes(item.status)"
                      variant="ghost"
                      size="sm"
                      :disabled="starting.has(item.id)"
                      @click="start(item)"
                    >
                      <Spinner v-if="starting.has(item.id)" data-icon="inline-start" />
                      启动
                    </Button>
                    <ConfirmAction
                      v-if="auth.canWrite"
                      :title="`确认删除 ${item.name}？`"
                      :disabled="deleting.has(item.id)"
                      @confirm="remove(item)"
                    >
                      <Button variant="destructiveGhost" size="sm" :disabled="deleting.has(item.id)">
                        <Spinner v-if="deleting.has(item.id)" data-icon="inline-start" />
                        删除
                      </Button>
                    </ConfirmAction>
                  </div>
                </TableCell>
              </TableRow>
              <TableEmpty v-if="filtered.length === 0" :colspan="8">
                <SbEmptyState title="还没有云沙盒" description="新建后按需启动；空闲后会自动回收" />
              </TableEmpty>
            </SbAsyncRegion>
          </Table>
        </div>
      </Card>
      <SandboxCreateModal v-model:open="createOpen" :project-id="projectId" :capabilities="capabilities" @saved="load" />
      <SandboxDrawer :open="!!selected" :project-id="projectId" :sandbox="selected" :max-file-bytes="capabilities?.maxFileBytes || 1048576" @update:open="!$event && (selected = null)" @changed="load" />
    </PageContainer>
  </ProjectScope>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
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

const store = useProjectStore()
const auth = useAuthStore()
const { projectId, isAdmin: isAdminProject } = storeToRefs(store)
const capabilities = ref<SandboxCapabilities | null>(null)
const records = ref<SandboxItem[]>([])
const statusFilter = ref('')
const sourceFilter = ref('')
const createOpen = ref(false)
const selected = ref<SandboxItem | null>(null)
const starting = ref(new Set<string>())
const stopping = ref(new Set<string>())
const deleting = ref(new Set<string>())
const { pending, settled, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: '加载云沙盒失败'
})

const filtered = computed(() => records.value.filter((s) =>
  (!statusFilter.value || s.status === statusFilter.value) && (!sourceFilter.value || s.source === sourceFilter.value)))

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

async function start(item: SandboxItem) {
  await runRow(starting, item.id, async () => {
    try {
      await api.sandboxes.start(projectId.value, item.id)
      await load()
      toast.success('沙盒已启动')
    } catch (e) {
      toast.error(errorMessage(e, '启动失败'))
    }
  })
}

async function stop(item: SandboxItem) {
  await runRow(stopping, item.id, async () => {
    try {
      await api.sandboxes.stop(projectId.value, item.id)
      await load()
      toast.success('沙盒已停止')
    } catch (e) {
      toast.error(errorMessage(e, '停止失败'))
    }
  })
}

async function remove(item: SandboxItem) {
  await runRow(deleting, item.id, async () => {
    try {
      await api.sandboxes.remove(projectId.value, item.id)
      await load()
      toast.success('沙盒已删除')
    } catch (e) {
      toast.error(errorMessage(e, '删除失败'))
    }
  })
}

void load()
watch(projectId, () => {
  void load()
})
</script>
