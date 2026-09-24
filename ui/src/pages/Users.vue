<template>
  <PageContainer subtitle="管理系统内全部账号与角色">
    <Card>
      <CardHeader class="border-b">
        <CardTitle>用户</CardTitle>
        <CardDescription>superadminl1 可增删改；admin 只读查看</CardDescription>
      </CardHeader>
      <CardContent class="flex flex-wrap items-end gap-2.5 border-b py-4">
        <Button v-if="auth.canManageUsers" size="sm" @click="openCreate">
          <PlusIcon data-icon="inline-start" />
          新建用户
        </Button>
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">角色</span>
          <Select v-model="roleFilter">
            <SelectTrigger class="w-32" size="sm">
              <SelectValue placeholder="全部" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="superadminl1">超管</SelectItem>
              <SelectItem value="admin">管理员</SelectItem>
              <SelectItem value="user">普通用户</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="flex flex-col gap-1">
          <span class="text-xs text-muted-foreground">状态</span>
          <Select v-model="statusFilter">
            <SelectTrigger class="w-28" size="sm">
              <SelectValue placeholder="全部" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="active">启用</SelectItem>
              <SelectItem value="disabled">禁用</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div class="flex min-w-55 flex-1 flex-col gap-1 sm:max-w-80">
          <span class="text-xs text-muted-foreground">搜索</span>
          <Input v-model="keyword" placeholder="搜索用户名" />
        </div>
        <Button size="sm" :disabled="loading" @click="load">
          <Spinner v-if="loading" data-icon="inline-start" />
          <RefreshCwIcon v-else data-icon="inline-start" />
          刷新
        </Button>
      </CardContent>
      <div class="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="sb-col-name">用户名</TableHead>
              <TableHead class="sb-col-sm">角色</TableHead>
              <TableHead class="sb-col-sm">状态</TableHead>
              <TableHead class="w-20 text-right">项目数</TableHead>
              <TableHead class="sb-col-md">最后登录</TableHead>
              <TableHead class="w-48">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <template v-if="loading && !filtered.length">
              <TableRow v-for="n in 3" :key="'sk-' + n">
                <TableCell colspan="6"><Skeleton class="h-8 w-full" /></TableCell>
              </TableRow>
            </template>
            <TableEmpty v-else-if="!filtered.length" :colspan="6">
              <SbEmptyState
                description="暂无用户"
                :action-text="auth.canManageUsers ? '新建用户' : undefined"
                @action="openCreate"
              />
            </TableEmpty>
            <TableRow v-for="u in filtered" :key="u.id">
              <TableCell class="font-medium">
                {{ u.username }}
                <span v-if="u.displayName" class="ml-1 text-xs text-muted-foreground">{{ u.displayName }}</span>
              </TableCell>
              <TableCell>{{ roleText(u.role) }}</TableCell>
              <TableCell>
                <span :class="u.status === 'active' ? 'text-success' : 'text-muted-foreground'">
                  {{ u.status === 'active' ? '● 启用' : '○ 禁用' }}
                </span>
              </TableCell>
              <TableCell>{{ u.projectCount }}</TableCell>
              <TableCell class="text-xs text-muted-foreground">{{ fmtTime(u.lastLoginAt) }}</TableCell>
              <TableCell>
                <div class="flex gap-1">
                  <Button size="sm" variant="outline" @click="openEdit(u)">查看</Button>
                  <template v-if="auth.canManageUsers && u.id !== auth.user?.id && u.username !== 'simplebase2026'">
                    <Button size="sm" variant="outline" @click="openEdit(u)">编辑</Button>
                    <Button size="sm" variant="outline" @click="toggleStatus(u)">
                      {{ u.status === 'active' ? '禁用' : '启用' }}
                    </Button>
                  </template>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
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
import { computed, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableEmpty,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import { Spinner } from '@/components/ui/spinner'
import PageContainer from '../components/PageContainer.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import UserFormModal from '../components/modal/UserFormModal.vue'
import { api } from '../services/api'
import type { UserItem, UserRole } from '../services/types'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const users = ref<UserItem[]>([])
const loading = ref(false)
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
  if (r === 'superadminl1') return '超管'
  if (r === 'admin') return '管理员'
  return '普通用户'
}

function fmtTime(s?: string) {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

async function load() {
  loading.value = true
  try {
    const res = await api.users.list(100)
    users.value = res.users
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
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
    toast.success(u.status === 'active' ? '已禁用' : '已启用')
    await load()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '操作失败')
  }
}

function onSaved() {
  formOpen.value = false
  void load()
}

onMounted(load)
</script>
