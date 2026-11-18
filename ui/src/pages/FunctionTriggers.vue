<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.functionTriggers.subtitle')">
      <SbListCard
        :title="t('pages.functionTriggers.cardTitle')"
        :description="t('pages.functionTriggers.cardDesc', { n: total })"
        :pager="{ page, pageSize, total, pageCount }"
        @update:page="page = $event"
      >
        <template #actions>
          <SbRefreshButton :pending="pending" @click="load" />
          <Button v-if="canWrite" size="sm" @click="openCreate">
            <PlusIcon data-icon="inline-start" />
            {{ t('pages.functionTriggers.create') }}
          </Button>
        </template>
        <div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="min-w-40">{{ t('pages.functionTriggers.kind') }}</TableHead>
                <TableHead class="sb-col-name">{{ t('pages.functionTriggers.target') }}</TableHead>
                <TableHead class="sb-col-name">{{ t('pages.functionTriggers.function') }}</TableHead>
                <TableHead class="sb-col-sm">{{ t('common.enabledSw') }}</TableHead>
                <TableHead class="sb-col-act min-w-40">{{ t('common.actions') }}</TableHead>
              </TableRow>
            </TableHeader>
            <SbAsyncRegion
              as="tbody"
              :columns="5"
              :pending="pending"
              :show-skeleton="showSkeleton"
              :show-empty="showEmpty"
              :show-error="showError"
              :refreshing="refreshing"
              :error="error"
              :empty-title="isAdmin ? t('pages.functionTriggers.emptyAdminTitle') : t('pages.functionTriggers.empty')"
              :empty-description="isAdmin ? t('pages.functionTriggers.emptyAdminDesc') : t('pages.functionTriggers.emptyDesc')"
              :empty-action-text="canWrite ? t('pages.functionTriggers.create') : undefined"
              @retry="load"
              @empty-action="openCreate"
            >
              <TableRow v-for="item in paged" :key="item.id">
                <TableCell>
                  <div class="flex flex-col gap-0.5">
                    <span class="sb-mono text-xs font-medium">{{ item.kind }}</span>
                    <span class="text-xs text-muted-foreground">{{ item.events.join(',') }}</span>
                  </div>
                </TableCell>
                <TableCell>
                  <div class="flex max-w-80 flex-col gap-0.5">
                    <span class="sb-mono truncate text-xs" :title="item.target">{{ item.target || '—' }}</span>
                    <span v-if="item.filter" class="truncate text-xs text-muted-foreground" :title="item.filter">{{ item.filter }}</span>
                  </div>
                </TableCell>
                <TableCell><span class="sb-mono text-sm">{{ item.funcFile }}.{{ item.funcExport }}</span></TableCell>
                <TableCell>
                  <Switch
                    :model-value="item.enabled"
                    :disabled="!auth.canWrite || toggling.has(item.id)"
                    @update:model-value="toggle(item, $event)"
                  />
                </TableCell>
                <TableCell>
                  <div v-if="auth.canWrite" class="flex flex-wrap justify-center gap-1">
                    <ConfirmAction
                      v-if="item.kind === 'http' && item.enabled"
                      :title="t('pages.functionTriggers.invokeConfirm')"
                      @confirm="invoke(item)"
                    >
                      <Button size="sm" variant="ghost" :disabled="invoking.has(item.id)">
                        <Spinner v-if="invoking.has(item.id)" data-icon="inline-start" />
                        {{ t('pages.functionTriggers.invoke') }}
                      </Button>
                    </ConfirmAction>
                    <ConfirmAction :title="t('pages.functionTriggers.deleteConfirm')" @confirm="remove(item)">
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
      </SbListCard>

      <FunctionTriggerModal v-model:open="modalOpen" :project-id="projectId" @saved="load" />
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { toast } from 'vue-sonner'
import { PlusIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { FunctionTrigger } from '@/services/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { errorMessage } from '@/utils/format'
import { useLoadState } from '@/composables/useLoadState'
import { usePagination } from '@/composables/usePagination'
import PageContainer from '@/components/PageContainer.vue'
import ProjectScope from '@/components/ProjectScope.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbRefreshButton from '@/components/SbRefreshButton.vue'
import SbListCard from '@/components/SbListCard.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import FunctionTriggerModal from '@/components/modal/FunctionTriggerModal.vue'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const { t } = useI18n()
const project = useProjectStore()
const auth = useAuthStore()
const { projectId, isAdmin } = storeToRefs(project)
/** 写入口：系统项目与只读角色都隐藏（planv6.7 ui-simplify-ux-plan S2）。 */
const canWrite = computed(() => !isAdmin.value && auth.canWrite)
const records = ref<FunctionTrigger[]>([])
const toggling = ref(new Set<string>())
const invoking = ref(new Set<string>())
const removing = ref(new Set<string>())
const modalOpen = ref(false)
let listRequest = 0

const { page, pageSize, total, pageCount, items: paged } = usePagination(records)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.functionTriggers.loadFailed'
})

function load() {
  const currentProject = projectId.value
  const request = ++listRequest
  return run(async () => {
    if (!currentProject || isAdmin.value) {
      records.value = []
      page.value = 1
      return false
    }
    const data = await api.triggers.list(currentProject)
    if (request !== listRequest || currentProject !== projectId.value) return records.value.length > 0
    records.value = data || []
    return records.value.length > 0
  })
}
function openCreate() {
  if (isAdmin.value || !auth.canWrite) return
  modalOpen.value = true
}
async function toggle(item: FunctionTrigger, enabled: boolean) {
  if (!auth.canWrite || toggling.value.has(item.id)) return
  toggling.value.add(item.id)
  try {
    const updated = await api.triggers.patch(projectId.value, item.id, { enabled })
    Object.assign(item, updated)
    toast.success(t(updated.enabled ? 'pages.functionTriggers.enabledToast' : 'pages.functionTriggers.pausedToast'))
  } catch (error) {
    toast.error(errorMessage(error, t('pages.functionTriggers.toggleFailed')))
  } finally {
    toggling.value.delete(item.id)
  }
}
async function invoke(item: FunctionTrigger) {
  if (!auth.canWrite || invoking.value.has(item.id)) return
  invoking.value.add(item.id)
  try {
    await api.triggers.invoke(projectId.value, item.id, {})
    toast.success(t('pages.functionTriggers.invoked'))
  } catch (error) {
    toast.error(errorMessage(error, t('pages.functionTriggers.invokeFailed')))
  } finally {
    invoking.value.delete(item.id)
  }
}
async function remove(item: FunctionTrigger) {
  if (!auth.canWrite || removing.value.has(item.id)) return
  removing.value.add(item.id)
  try {
    await api.triggers.remove(projectId.value, item.id)
    toast.success(t('pages.functionTriggers.deleted'))
    await load()
  } catch (error) {
    toast.error(errorMessage(error, t('pages.functionTriggers.deleteFailed')))
  } finally {
    removing.value.delete(item.id)
  }
}

watch(projectId, () => {
  modalOpen.value = false
  page.value = 1
  void load()
}, { immediate: true })
</script>
