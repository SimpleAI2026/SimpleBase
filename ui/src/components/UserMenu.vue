<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="sm" class="gap-2" aria-label="账号菜单">
        <span class="flex size-6 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
          {{ avatarChar }}
        </span>
        <span class="hidden max-w-24 truncate text-sm sm:inline">{{ auth.displayName }}</span>
        <Badge v-if="roleLabel" variant="secondary" class="hidden md:inline-flex">{{ roleLabel }}</Badge>
      </Button>
    </PopoverTrigger>
    <PopoverContent class="w-56 p-0" align="end">
      <div class="flex flex-col gap-1 p-1">
        <div class="px-2 py-1.5">
          <p class="text-sm font-medium">{{ auth.displayName }}</p>
          <p class="text-xs text-muted-foreground">{{ auth.user?.username }}</p>
        </div>
        <Separator />
        <Button variant="ghost" class="w-full justify-start" @click="openPassword">
          <KeyIcon data-icon="inline-start" />
          修改密码
        </Button>
        <Button variant="ghost" class="w-full justify-start" @click="doLogout">
          <LogOutIcon data-icon="inline-start" />
          退出登录
        </Button>
      </div>
    </PopoverContent>
  </Popover>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { KeyIcon, LogOutIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const open = ref(false)

const avatarChar = computed(() => (auth.displayName || '?').slice(0, 1).toUpperCase())
const roleLabel = computed(() => {
  switch (auth.role) {
    case 'superadminl1':
      return '超管'
    case 'admin':
      return '管理员'
    case 'user':
      return ''
    default:
      return ''
  }
})

function openPassword() {
  open.value = false
  auth.mustChangePassword = true
  auth.openLogin()
}

async function doLogout() {
  open.value = false
  await auth.logout()
}
</script>
