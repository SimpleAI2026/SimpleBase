<template>
  <div>
    <Popover v-model:open="open">
      <PopoverTrigger as-child>
        <Button variant="outline" class="h-8 max-w-60 justify-start gap-2 px-2.5" aria-label="切换项目">
          <FolderIcon />
          <span class="flex min-w-0 flex-col items-start leading-tight">
            <span class="max-w-40 truncate text-sm font-semibold">{{ store.displayName }}</span>
            <span v-if="store.id" class="font-mono text-[11px] text-muted-foreground">{{ shortId(store.id) }}</span>
          </span>
          <ChevronDownIcon class="ml-auto opacity-60" />
        </Button>
      </PopoverTrigger>
      <PopoverContent class="w-[300px] p-0" align="end">
        <Command>
          <CommandInput placeholder="搜索项目名称或 ID" />
          <CommandList>
            <CommandEmpty>{{ store.loading ? '加载项目…' : '没有匹配的项目' }}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                v-for="p in merged"
                :key="p.id"
                :value="`${p.name} ${p.id}`"
                @select="() => select(p)"
              >
                <span class="flex min-w-0 flex-col">
                  <span class="truncate">{{ p.name || p.id }}</span>
                  <span class="font-mono text-[11px] text-muted-foreground break-all">{{ p.id }}</span>
                </span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
        <Separator />
        <div class="p-1">
          <Button variant="ghost" class="w-full justify-start" @click="openCreate">
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
import { ChevronDownIcon, FolderIcon, PlusIcon } from '@lucide/vue'
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
import { useProjectStore } from '../stores/project'
import type { ProjectItem } from '../services/types'
import CreateProjectModal from './modal/CreateProjectModal.vue'

const store = useProjectStore()
const open = ref(false)

function shortId(id: string) {
  const compact = id.replace(/-/g, '')
  if (compact.length >= 8) return compact.slice(-8)
  return id.length > 8 ? id.slice(0, 8) : id
}

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
