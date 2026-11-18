<template>
  <SbListCard
    :title="t('components.mcpServers.cardTitle')"
    :description="t('components.mcpServers.cardDesc', { n: records.length })"
  >
    <template #actions>
      <SbRefreshButton :pending="loading" @click="load" />
      <Button v-if="auth.canWrite" size="sm" @click="openCreate">
        <PlusIcon data-icon="inline-start" />
        {{ t('components.mcpServers.create') }}
      </Button>
    </template>
    <div class="p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="sb-col-name">{{ t('components.mcpServers.colName') }}</TableHead>
            <TableHead class="min-w-48">{{ t('components.mcpServers.colUrl') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('components.mcpServers.colCredential') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('components.mcpServers.colTools') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('common.enabledSw') }}</TableHead>
            <TableHead class="sb-col-act min-w-48">{{ t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="6"
          :pending="loading"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-title="t('components.mcpServers.emptyTitle')"
          :empty-description="t('components.mcpServers.emptyDesc')"
          @retry="load"
        >
          <TableRow v-for="item in records" :key="item.id">
            <TableCell>
              <div class="flex flex-col gap-0.5">
                <span class="sb-mono text-sm font-medium">{{ item.name }}</span>
                <span class="text-xs text-muted-foreground">{{ item.transport }}</span>
              </div>
            </TableCell>
            <TableCell>
              <span class="sb-mono truncate text-xs" :title="item.url">{{ item.url }}</span>
            </TableCell>
            <TableCell>
              <Badge v-if="item.has_credential" variant="secondary">
                {{ item.header_names.join(', ') }}
              </Badge>
              <span v-else class="text-xs text-muted-foreground">—</span>
            </TableCell>
            <TableCell>
              <span class="sb-mono text-xs">{{ toolCount(item) }}</span>
            </TableCell>
            <TableCell>
              <Switch
                :model-value="item.enabled"
                :disabled="!auth.canWrite || toggling.has(item.id)"
                @update:model-value="toggle(item, $event)"
              />
            </TableCell>
            <TableCell>
              <div v-if="auth.canWrite" class="flex flex-wrap justify-center gap-1">
                <Button size="sm" variant="ghost" :disabled="testing.has(item.id)" @click="test(item)">
                  <Spinner v-if="testing.has(item.id)" data-icon="inline-start" />
                  {{ t('components.mcpServers.test') }}
                </Button>
                <Button size="sm" variant="ghost" @click="openEdit(item)">
                  {{ t('common.edit') }}
                </Button>
                <ConfirmAction :title="t('components.mcpServers.deleteConfirm', { name: item.name })" @confirm="remove(item)">
                  <Button size="sm" variant="destructiveGhost" :disabled="removing.has(item.id)">
                    <Spinner v-if="removing.has(item.id)" data-icon="inline-start" />
                    {{ t('common.del') }}
                  </Button>
                </ConfirmAction>
              </div>
              <span v-else class="text-xs text-muted-foreground">—</span>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>

    <McpServerModal
      v-model:open="modalOpen"
      :project-id="projectId"
      :server="editing"
      @saved="load"
    />
  </SbListCard>
</template>

<script setup lang="ts">
/**
 * MCP Server 配置面板（planv6.2 §6-7）。
 *
 * 挂在「设置」页：项目级配置，与模型设置同级。
 * 凭据只单向写：列表只展示 header 名字，编辑时留空即沿用既有值。
 */
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { McpServer } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { errorMessage } from '@/utils/format'
import { useLoadState } from '@/composables/useLoadState'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbRefreshButton from '@/components/SbRefreshButton.vue'
import SbListCard from '@/components/SbListCard.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import McpServerModal from '@/components/modal/McpServerModal.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const { t } = useI18n()
const project = useProjectStore()
const auth = useAuthStore()
const { projectId } = storeToRefs(project)

const records = ref<McpServer[]>([])
const toggling = ref(new Set<string>())
const testing = ref(new Set<string>())
const removing = ref(new Set<string>())
const modalOpen = ref(false)
const editing = ref<McpServer | null>(null)
let listRequest = 0

const { pending: loading, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.mcpServers.loadFailed'
})

function toolCount(item: McpServer): number {
  return Object.keys(item.tools_policy || {}).length
}

function load() {
  const current = projectId.value
  const request = ++listRequest
  return run(async () => {
    if (!current) {
      records.value = []
      return false
    }
    const list = await api.mcpServers.list(current)
    if (request !== listRequest || current !== projectId.value) return records.value.length > 0
    records.value = list || []
    return records.value.length > 0
  })
}

function openCreate() {
  if (!auth.canWrite) return
  editing.value = null
  modalOpen.value = true
}

function openEdit(item: McpServer) {
  if (!auth.canWrite) return
  editing.value = item
  modalOpen.value = true
}

async function toggle(item: McpServer, enabled: boolean) {
  if (!auth.canWrite || toggling.value.has(item.id)) return
  toggling.value.add(item.id)
  try {
    const updated = await api.mcpServers.update(projectId.value, item.id, { enabled })
    Object.assign(item, updated)
    toast.success(t('components.mcpServers.saved'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServers.saveFailed')))
  } finally {
    toggling.value.delete(item.id)
  }
}

async function test(item: McpServer) {
  if (testing.value.has(item.id)) return
  testing.value.add(item.id)
  try {
    const res = await api.mcpServers.test(projectId.value, item.id)
    if (res.ok) {
      toast.success(t('components.mcpServers.testOk', { n: res.tools.length }))
    } else {
      // 错误只回稳定分类，不回上游原文。
      toast.error(t('components.mcpServers.testFailed'))
    }
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServers.testFailed')))
  } finally {
    testing.value.delete(item.id)
  }
}

async function remove(item: McpServer) {
  if (!auth.canWrite || removing.value.has(item.id)) return
  removing.value.add(item.id)
  try {
    await api.mcpServers.remove(projectId.value, item.id)
    toast.success(t('components.mcpServers.deleted'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServers.deleteFailed')))
  } finally {
    removing.value.delete(item.id)
  }
}

watch(
  projectId,
  () => {
    modalOpen.value = false
    editing.value = null
    void load()
  },
  { immediate: true }
)
</script>