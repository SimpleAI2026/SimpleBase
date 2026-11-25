<template>
  <div class="flex flex-col gap-3">
    <SbAsyncRegion class="flex flex-col gap-3" block="spinner" :state="state" @retry="load">
      <div v-if="!elems.length" class="rounded-md border border-dashed border-border/70 py-6 text-center text-xs text-muted-foreground">
        {{ t('components.kvSet.empty') }}
      </div>
      <div v-else class="flex flex-wrap gap-1.5">
        <Badge v-for="el in elems" :key="el" variant="secondary" class="sb-mono text-xs">
          {{ el }}
          <button
            v-if="!readonly"
            class="ml-1 opacity-60 hover:opacity-100"
            :title="t('components.kvSet.remove')"
            @click="removeMember(el)"
          >×</button>
        </Badge>
      </div>
      <div v-if="!readonly" class="flex flex-wrap items-center gap-2">
        <Input v-model="newMember" :placeholder="t('components.kvSet.memberPlaceholder')" class="min-w-0 flex-1 sm:w-48 sm:flex-none" @keydown.enter="addMember" />
        <Button size="sm" :disabled="!newMember.trim()" @click="addMember">{{ t('components.kvSet.add') }}</Button>
        <Button
          size="sm"
          variant="outline"
          class="ml-auto"
          :disabled="!elems.length"
          @click="popRandom"
        >{{ t('components.kvSet.randomPop') }}</Button>
      </div>
    </SbAsyncRegion>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import SbAsyncRegion from '../../../SbAsyncRegion.vue'
import { useKvEditor } from '@/composables/useKvEditor'

const props = defineProps<{
  projectId: string
  kvKey: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()

const elems = ref<string[]>([])
const newMember = ref('')

const { state, load, mutate } = useKvEditor({
  projectId: () => props.projectId,
  kvKey: () => props.kvKey,
  fallback: 'components.kvSet.loadFailed',
  read: () => ({ type: 'cmd', argvs: ['SMEMBERS', props.kvKey] }),
  parse: (r) => (Array.isArray(r) ? (r as string[]) : []),
  apply: (list) => {
    elems.value = list
  },
  changed: () => emit('changed')
})

function addMember() {
  const v = newMember.value.trim()
  if (!v) return
  void mutate({ type: 'Set', args: { key: props.kvKey, elems: [v] } }, {
    failKey: 'components.kvSet.addFailed',
    onSuccess: () => {
      newMember.value = ''
    }
  })
}

function removeMember(v: string) {
  void mutate({ type: 'cmd', argvs: ['SREM', props.kvKey, v] }, { failKey: 'components.kvSet.removeFailed' })
}

function popRandom() {
  void mutate({ type: 'cmd', argvs: ['SPOP', props.kvKey] }, {
    failKey: 'components.kvSet.popFailed',
    onSuccess: (r) => toast.success(t('components.kvSet.popped', { value: r ?? '' }))
  })
}
</script>
