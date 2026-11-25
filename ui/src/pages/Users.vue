<template>
  <PageContainer :subtitle="t('pages.users.subtitle')">
    <SbListCard
      :title="t('pages.users.cardTitle')"
      :description="t('pages.users.cardDesc')"
      :pager="{ page, pageSize, total, pageCount }"
      @update:page="goToPage"
    >
      <template #actions>
        <SbRefreshButton :pending="pending" @click="load" />
        <Button v-if="auth.canManageUsers" size="sm" @click="openCreate">
          <PlusIcon data-icon="inline-start" />
          {{ t('pages.users.create') }}
        </Button>
      </template>
      <template #toolbar>
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
      </template>
      <div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="sb-col-name">{{ t('pages.users.colUsername') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.users.colRole') }}</TableHead>
              <TableHead class="sb-col-sm">{{ t('pages.users.colStatus') }}</TableHead>
              <TableHead class="w-20">{{ t('pages.users.colProjects') }}</TableHead>
              <TableHead class="sb-col-md">{{ t('pages.users.colLastLogin') }}</TableHead>
              <TableHead class="sb-col-act">{{ t('pages.users.colActions') }}</TableHead>
            </TableRow>
          </TableHeader>
          <SbAsyncRegion
            as="tbody"
            :columns="6"
            :state="loadState"
            :empty-description="t('pages.users.emptyDesc')"
            :empty-action-text="auth.canManageUsers ? t('pages.users.create') : undefined"
            @retry="load"
            @empty-action="openCreate"
          >
            <TableEmpty v-if="hasData && !filtered.length" :colspan="6">
              <SbEmptyState
                :description="t('common.noMatchFilter')"
                :action-text="t('common.clearFilter')"
                :action-icon="XIcon"
                @action="clearFilters"
              />
            </TableEmpty>
            <TableRow v-for="u in filteredPage" :key="u.id">
              <TableCell class="font-medium">
                {{ u.username }}
                <span v-if="u.displayName" class="ml-1 text-xs text-muted-foreground">{{ u.displayName }}</span>
              </TableCell>
              <TableCell>{{ roleText(u.role) }}</TableCell>
              <TableCell>
                <Badge :variant="u.status === 'active' ? 'success' : 'outline'">
                  {{ u.status === 'active' ? t('pages.users.activeBadge') : t('pages.users.disabledBadge') }}
                </Badge>
              </TableCell>
              <TableCell>{{ u.projectCount }}</TableCell>
              <TableCell class="text-xs text-muted-foreground">{{ formatTime(u.lastLoginAt, '—') }}</TableCell>
              <TableCell>
                <div class="flex flex-wrap items-center justify-center gap-1">
                  <!-- 「查看」与「编辑」打开同一个弹窗：可管理时只给「编辑」 -->
                  <Button size="sm" variant="ghost" @click="openEdit(u)">
                    {{ canManage(u) ? t('pages.users.edit') : t('pages.users.view') }}
                  </Button>
                  <Button v-if="canManage(u)" size="sm" variant="ghost" @click="toggleStatus(u)">
                    {{ u.status === 'active' ? t('pages.users.disable') : t('pages.users.enable') }}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </SbAsyncRegion>
        </Table>
      </div>
    </SbListCard>

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
import { PlusIcon, XIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import PageContainer from '@/components/PageContainer.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbRefreshButton from '@/components/SbRefreshButton.vue'
import SbListCard from '@/components/SbListCard.vue'
import SbEmptyState from '@/components/SbEmptyState.vue'
import UserFormModal from '@/components/modal/UserFormModal.vue'
import { api } from '@/services/api'
import type { UserItem, UserRole } from '@/services/types'
import { BOOTSTRAP_USERNAME, useAuthStore } from '@/stores/auth'
import { useLoadState } from '@/composables/useLoadState'
import { formatTime } from '@/utils/format'

const auth = useAuthStore()
const { t } = useI18n()
const users = ref<UserItem[]>([])
const keyword = ref('')
/** 游标分页（后端 /v1/users 按 cursor 翻页，上限 100/页）。 */
const page = ref(1)
const pageSize = 20
const cursors = ref<string[]>([''])
const nextCursor = ref('')
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
/** 当前页展示的行：筛选项仍作用于整页数据。 */
const filteredPage = computed(() => filtered.value.slice(0, pageSize))
const total = computed(() => (page.value - 1) * pageSize + filtered.value.length + (nextCursor.value ? 1 : 0))
const pageCount = computed(() => (nextCursor.value ? page.value + 1 : page.value))

function roleText(r: UserRole | string) {
  if (r === 'superadminl1') return t('components.userMenu.roleSuper')
  if (r === 'admin') return t('components.userMenu.roleAdmin')
  return t('pages.users.roleUser')
}

/** 可管理（编辑 / 启停）：有管理权限、不是自己、不是受保护的引导账号。 */
function canManage(u: UserItem): boolean {
  return auth.canManageUsers && u.id !== auth.user?.id && u.username !== BOOTSTRAP_USERNAME
}

function clearFilters() {
  keyword.value = ''
  roleFilter.value = 'all'
  statusFilter.value = 'all'
}

function goToPage(value: number) {
  if (value < 1 || value > pageCount.value || value === page.value) return
  page.value = value
  void load()
}

const loadState = useLoadState({
  fallback: 'pages.users.loadFailed'
})
const { pending, hasData, run } = loadState

function load() {
  return run(async () => {
    const data = await api.users.list(pageSize, cursors.value[page.value - 1] || undefined)
    users.value = data.users
    nextCursor.value = data.nextCursor || ''
    if (nextCursor.value) cursors.value[page.value] = nextCursor.value
    return data.users.length > 0
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
