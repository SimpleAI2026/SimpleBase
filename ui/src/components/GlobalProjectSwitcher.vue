<template>
  <div class="w-32 min-w-0 shrink sm:w-auto sm:shrink-0">
    <Popover v-model:open="open">
      <PopoverTrigger as-child>
        <Button
          variant="outline"
          class="h-9 w-full min-w-0 max-w-full justify-start gap-1 rounded-lg px-2 shadow-xs sm:w-56 sm:gap-2.5 sm:px-3"
          :aria-label="t('components.projectSwitcher.aria', { label: triggerLabel })"
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
              :placeholder="t('components.projectSwitcher.searchPlaceholder')"
            />
          </div>
          <CommandList class="max-h-[340px] px-2 py-2">
            <CommandEmpty class="py-8 text-center text-sm text-muted-foreground">
              {{ store.loading ? t('components.projectSwitcher.loading') : t('components.projectSwitcher.noMatch') }}
            </CommandEmpty>

            <CommandGroup v-if="projects.length" class="p-0">
              <CommandItem
                v-for="p in projects"
                :key="p.id"
                :value="`${p.name || p.id} ${p.id}`"
                class="mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-2 last:mb-0"
                @select="() => select(p)"
              >
                <ShieldIcon
                  v-if="isAdminProject(p)"
                  class="size-3.5 shrink-0 text-muted-foreground/60"
                />
                <span class="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span class="truncate text-sm font-medium">
                    {{ p.name || p.id }}
                    <span
                      v-if="isAdminProject(p)"
                      class="ml-1 text-xs font-normal text-muted-foreground"
                    >{{ t('components.projectSwitcher.readonlySuffix') }}</span>
                  </span>
                  <span class="sb-mono truncate text-[11px] tracking-wide text-muted-foreground/70">{{ p.id }}</span>
                </span>
                <span
                  v-if="isAdminProject(p)"
                  class="rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary"
                >
                  ★
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
            {{ t('components.projectSwitcher.create') }}
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
import { useI18n } from 'vue-i18n'
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
const { t } = useI18n()
const open = ref(false)

const triggerLabel = computed(() => {
  const name = store.displayName.trim()
  if (name && name !== store.id) return name
  return store.id || t('common.selectProject')
})

const merged = computed<ProjectItem[]>(() => store.projects.filter((p) => p.id))

const isAdminProject = (p: ProjectItem) => p.id === ADMIN_PROJECT_ID || p.managed

const projects = computed<ProjectItem[]>(() => {
  const users = merged.value.filter((p) => !isAdminProject(p))
  if (!auth.isSuper && !auth.isAdminRole) return users
  return [...merged.value.filter(isAdminProject), ...users]
})

watch(open, (next) => {
  if (next) {
    void store.loadProjects()
  }
})

function select(p: ProjectItem) {
  if (p.id !== store.id) {
    store.setProject(p.id, p.name)
    toast.success(t('components.projectSwitcher.switched', { name: p.name || p.id }))
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
