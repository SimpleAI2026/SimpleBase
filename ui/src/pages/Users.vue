<template>
  <PageContainer :subtitle="t('pages.users.subtitle')">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>{{ t('pages.users.cardTitle') }}</CardTitle>
        <CardDescription>{{ t('pages.users.cardDesc') }}</CardDescription>
        <CardAction>
          <div class="flex items-center gap-2">
            <Button variant="outline" size="sm" :disabled="pending" @click="load">
              <Spinner v-if="pending" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              {{ t('common.refresh') }}
            </Button>
            <Button v-if="auth.canManageUsers" size="sm" @click="openCreate">
              <PlusIcon data-icon="inline-start" />
              {{ t('pages.users.create') }}
            </Button>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent class="flex flex-wrap items-end gap-2.5 border-b py-4">
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">{{ t('pages.users.filterRole') }}</span>
          <Select v-model="roleFilter">
            <SelectTrigger class="w-32" size="sm">
              <SelectValue :placeholder="t('common.all')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ t('common.all') }}</SelectItem>
              <SelectItem value="superadminl1">{{ t('components.userMenu.roleSuper') }}</SelectItem>
              <SelectItem value="admin">{{ t('components.userMenu.roleAdmin') }}</SelectItem>
              <SelectItem value="user">{{ t('pages.users.roleUser') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">{{ t('pages.users.filterStatus') }}</span>
          <Select v-model="statusFilter">
            <SelectTrigger class="w-28" size="sm">
              <SelectValue :placeholder="t('common.all')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{{ t('common.all') }}</SelectItem>
              <SelectItem value="active">{{ t('common.enabledState') }}</SelectItem>
              <SelectItem value="disabled">{{ t('common.disabledState') }}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="flex min-w-55 flex-1 flex-col gap-1 sm:max-w-80">
          <span class="text-xs text-muted-foreground">{{ t('pages.users.filterSearch') }}</span>
          <Input v-model="keyword" :placeholder="t('pages.users.searchPlaceholder')" />
        </div>
      </CardContent>
      <div class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="sb-col-name">{{ t('pages.users.colUsername') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.users.colRole') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.users.colStatus') }}</TableHead>
              <TableHead class="w-20">{{ t('pages.users.colProjects') }}</TableHead>
              <TableHead class="sb-col-md">{{ t('pages.users.colLastLogin') }}</TableHead>
              <TableHead class="w-48">{{ t('pages.users.colActions') }}</TableHead>
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
            :empty-description="t('pages.users.emptyDesc')"
            :empty-action-text="auth.canManageUsers ? t('pages.users.create') : undefined"
            @retry="load"
            @empty-action="openCreate"
          >
            <TableEmpty v-if="hasData && !filtered.length" :colspan="6">
              <SbEmptyState :description="t('pages.users.emptyDesc')" />
            </TableEmpty>
            <TableRow v-for="u in filtered" :key="u.id">
              <TableCell class="font-medium">
                {{ u.username }}
                <span v-if="u.displayName" class="ml-1 text-xs text-muted-foreground">{{ u.displayName }}</span>
              </TableCell>
              <TableCell>{{ roleText(u.role) }}</TableCell>
              <TableCell>
                <span :class="u.status === 'active' ? 'text-success' : 'text-muted-foreground'">
                  {{ u.status === 'active' ? t('pages.users.activeBadge') : t('pages.users.disabledBadge') }}
                </span>
              </TableCell>
              <TableCell>{{ u.projectCount }}</TableCell>
              <TableCell class="text-xs text-muted-foreground">{{ fmtTime(u.lastLoginAt) }}</TableCell>
              <TableCell>
                <div class="flex justify-center gap-1">
                  <Button size="sm" variant="outline" @click="openEdit(u)">{{ t('pages.users.view') }}</Button>
                  <template v-if="auth.canManageUsers && u.id !== auth.user?.id && u.username !== 'simplebase2026'">
                    <Button size="sm" variant="outline" @click="openEdit(u)">{{ t('pages.users.edit') }}</Button>
                    <Button size="sm" variant="outline" @click="toggleStatus(u)">
                      {{ u.status === 'active' ? t('pages.users.disable') : t('pages.users.enable') }}
                    </Button>
                  </template>
                </div>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
      </div>
    </Card>

    <UserFormModal
      :open="formOpen"
      :user="editing"
      @update:open="formOpen = $event"
      @saved="onSaved"
    />
  </PageContainer>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Table,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Spinner } from '@/components/ui/spinner'
import PageContainer from '../components/PageContainer.vue'
import SbAsyncRegion from '../components/SbAsyncRegion.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import UserFormModal from '../components/modal/UserFormModal.vue'
import { api } from '../services/api'
import type { UserItem, UserRole } from '../services/types'
import { useAuthStore } from '../stores/auth'
import { useLoadState } from '../composables/useLoadState'
import { formatTime } from '../utils/format'

const auth = useAuthStore()
const { t } = useI18n()
const users = ref<UserItem[]>([])
const keyword = ref('')
const roleFilter = ref<'all' | UserRole>('all')
const statusFilter = ref<'all' | UserItem['status']>('all')
const formOpen = ref(false)
const editing = ref<UserItem | null>(null)

const filtered = computed(() =>
  users.value.filter((u) => {
    if (roleFilter.value !== 'all' && u.role !== roleFilter.value) return false
    if (statusFilter.value !== 'all' && u.status !== statusFilter.value) return false
    const k = keyword.value.trim().toLowerCase()
    if (k && !u.username.toLowerCase().includes(k) && !(u.displayName || '').toLowerCase().includes(k))
      return false
    return true
  })
)

function roleText(r: UserRole | string) {
  if (r === 'superadminl1') return t('components.userMenu.roleSuper')
  if (r === 'admin') return t('components.userMenu.roleAdmin')
  return t('pages.users.roleUser')
}

function fmtTime(s?: string) {
  return formatTime(s, '—')
}

const { pending, hasData, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'pages.users.loadFailed'
})

function load() {
  return run(async () => {
    const data = await api.users.list(100)
    users.value = data.users
    return users.value.length > 0
  })
}

function openCreate() {
  editing.value = null
  formOpen.value = true
}

function openEdit(u: UserItem) {
  editing.value = u
  formOpen.value = true
}

async function toggleStatus(u: UserItem) {
  try {
    await api.users.update(u.id, { status: u.status === 'active' ? 'disabled' : 'active' })
    toast.success(u.status === 'active' ? t('pages.users.disabledToast') : t('pages.users.enabledToast'))
    await load()
  } catch (e) {
    toast.error(errorMessage(e, t('pages.users.failedToast')))
  }
}

function onSaved() {
  formOpen.value = false
  void load()
}

void load()
</script>
