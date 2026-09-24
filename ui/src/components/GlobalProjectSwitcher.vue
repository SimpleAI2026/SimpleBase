<template>
  <div>
    <Popover v-model:open="open">
      <PopoverTrigger as-child>
        <Button
          variant="outline"
          class="h-9 w-56 justify-start gap-2.5 rounded-lg px-3 shadow-xs"
          aria-label="切换项目"
        >
          <FolderIcon class="size-4 shrink-0 text-primary/80" />
          <span class="min-w-0 flex-1 truncate text-sm font-medium">{{ triggerLabel }}</span>
          <ChevronDownIcon class="ml-auto size-3.5 shrink-0 text-muted-foreground/60" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        class="w-[340px] max-w-[calc(100vw-1.5rem)] overflow-hidden rounded-xl p-0 shadow-lg"
        align="end"
        :side-offset="8"
      >
        <Command class="rounded-xl">
          <div class="border-b px-3 py-2.5">
            <CommandInput
              class="h-9 border-none bg-transparent px-0 shadow-none focus-visible:ring-0"
              placeholder="搜索项目名称或 ID"
            />
          </div>
          <CommandList class="max-h-[340px] px-2 py-2">
            <CommandEmpty class="py-8 text-center text-sm text-muted-foreground">
              {{ store.loading ? '加载项目…' : '没有匹配的项目' }}
            </CommandEmpty>

            <CommandGroup
              v-if="adminProjects.length"
              heading="admin 管理数据库"
              class="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground/70 [&_[cmdk-group-heading]]:uppercase"
            >
              <CommandItem
                v-for="p in adminProjects"
                :key="p.id"
                :value="`${p.name || p.id} ${p.id}`"
                class="mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-2 last:mb-0"
                @select="() => select(p)"
              >
                <ShieldIcon class="size-3.5 shrink-0 text-muted-foreground/60" />
                <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span class="truncate text-sm font-medium">
                    {{ p.name || p.id }}
                    <span class="ml-1 text-xs font-normal text-muted-foreground">（只读）</span>
                  </span>
                  <span class="sb-mono truncate text-[11px] tracking-wide text-muted-foreground/70">{{ p.id }}</span>
                </span>
                <CheckIcon
                  v-if="p.id === store.id"
                  class="size-4 shrink-0 text-primary"
                  :stroke-width="2.5"
                />
              </CommandItem>
            </CommandGroup>

            <CommandGroup
              :heading="auth.isSuper || auth.isAdminRole ? '全部项目' : '我的项目'"
              class="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:pt-1 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:text-muted-foreground/70 [&_[cmdk-group-heading]]:uppercase"
            >
              <CommandItem
                v-for="p in userProjects"
                :key="p.id"
                :value="`${p.name || p.id} ${p.id}`"
                class="mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-2 last:mb-0"
                @select="() => select(p)"
              >
                <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span class="truncate text-sm font-medium">{{ p.name || p.id }}</span>
                  <span class="sb-mono truncate text-[11px] tracking-wide text-muted-foreground/70">{{ p.id }}</span>
                </span>
                <CheckIcon
                  v-if="p.id === store.id"
                  class="size-4 shrink-0 text-primary"
                  :stroke-width="2.5"
                />
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
        <Separator />
        <div class="p-1.5">
          <Button
            v-if="auth.canCreateProject"
            variant="ghost"
            class="h-9 w-full justify-start gap-2 rounded-lg px-2.5 text-muted-foreground hover:text-foreground"
            @click="openCreate"
          >
            <PlusIcon data-icon="inline-start" />
            新建项目
          </Button>
        </div>
      </PopoverContent>
    </Popover>
    <CreateProjectModal
      :open="store.createModalOpen"
      @update:open="(v: boolean) => (v ? store.openCreateModal() : store.closeCreateModal())"
      @created="onCreated"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { CheckIcon, ChevronDownIcon, FolderIcon, PlusIcon, ShieldIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { useAuthStore } from '../stores/auth'
import { ADMIN_PROJECT_ID, useProjectStore } from '../stores/project'
import type { ProjectItem } from '../services/types'
import CreateProjectModal from './modal/CreateProjectModal.vue'

const store = useProjectStore()
const auth = useAuthStore()
const open = ref(false)

const triggerLabel = computed(() => {
  const name = store.displayName.trim()
  if (name && name !== store.id) return name
  return store.id || '选择项目'
})

const merged = computed<ProjectItem[]>(() => {
  const map = new Map<string, ProjectItem>()
  for (const p of store.projects) {
    if (p.id) map.set(p.id, p)
  }
  for (const id of store.history) {
    if (!map.has(id)) map.set(id, { id, name: id === store.id ? store.projectName : '', createdAt: '' })
  }
  if (store.id && !map.has(store.id)) {
    map.set(store.id, { id: store.id, name: store.projectName, createdAt: '' })
  }
  return Array.from(map.values())
})

const isAdminProject = (p: ProjectItem) => p.id === ADMIN_PROJECT_ID || p.managed

const adminProjects = computed<ProjectItem[]>(() =>
  auth.isSuper || auth.isAdminRole ? merged.value.filter(isAdminProject) : []
)
const userProjects = computed<ProjectItem[]>(() => merged.value.filter((p) => !isAdminProject(p)))

watch(open, (next) => {
  if (next) {
    void store.loadProjects()
  }
})

function select(p: ProjectItem) {
  if (p.id !== store.id) {
    store.setProject(p.id, p.name)
    toast.success(`已切换到「${p.name || p.id}」`)
  }
  open.value = false
}

async function openCreate() {
  open.value = false
  await nextTick()
  store.openCreateModal()
}

async function onCreated(p: ProjectItem) {
  store.setProject(p.id, p.name)
  await store.loadProjects()
}

watch(
  () => store.createModalOpen,
  (v) => {
    if (v) open.value = false
  }
)

onMounted(() => {
  void store.loadProjects()
})
</script>
