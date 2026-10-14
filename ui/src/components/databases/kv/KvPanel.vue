<template>
  <div class="flex flex-col gap-3 py-1">
    <Tabs v-model="tab" class="gap-1">
      <TabsList>
        <TabsTrigger class="px-3" value="data">{{ t('components.kvPanel.tabData') }}</TabsTrigger>
        <TabsTrigger class="px-3" value="api">API</TabsTrigger>
      </TabsList>
      <TabsContent value="data" class="mt-1 flex flex-col gap-3">
        <KvKeyListPanel
          v-model:pattern="pattern"
          v-model:type-filter="typeFilter"
          :rows="rows"
          :readonly="readonly"
          :has-more="hasMore"
          :loading-more="loadingMore"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :ttl-text="ttlText"
          @reload="reload"
          @create="openCreate"
          @retry="load"
          @load-more="loadMore"
          @detail="openDetail"
          @ttl="openTtl"
          @rename="openRename"
          @remove="removeKey"
        />
      </TabsContent>

      <TabsContent value="api" class="mt-1">
        <KvApiPanel :project-id="projectId" :readonly="readonly" @changed="reload" />
      </TabsContent>
    </Tabs>

    <KvCreateKeyModal v-model:open="createOpen" :project-id="projectId" :readonly="readonly" @created="onCreated" />
    <KvTtlModal v-model:open="ttlOpen" :project-id="projectId" :kv-key="ttlTarget" @changed="reload" />
    <KvDetailModal
      v-model:open="detailOpen"
      :project-id="projectId"
      :kv-key="detailTarget"
      :readonly="readonly"
      @changed="reload"
      @deleted="onDeleted"
    />

    <SbModal
      :open="renameOpen"
      :title="t('components.kvPanel.renameTitle')"
      :width="440"
      :confirm-loading="renaming"
      :ok-button-props="{ disabled: !renameValue.trim() }"
      @ok="submitRename"
      @update:open="renameOpen = $event"
    >
      <FieldGroup>
        <Field>
          <FieldLabel for="kv-rename">{{ t('components.kvPanel.newName') }}</FieldLabel>
          <Input id="kv-rename" v-model="renameValue" :placeholder="t('components.kvPanel.newNamePlaceholder')" @keydown.enter="submitRename" />
        </Field>
      </FieldGroup>
    </SbModal>
  </div>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useLoadState } from '@/composables/useLoadState'
import { isEmptyResourceError } from '@/utils/api-errors'
import { api } from '../../../services/api'
import type { KvKeyMeta } from '../../../services/api'
import KvKeyListPanel from './KvKeyListPanel.vue'
import KvApiPanel from './KvApiPanel.vue'
import SbModal from '../../modal/SbModal.vue'
import KvCreateKeyModal from './KvCreateKeyModal.vue'
import KvTtlModal from './KvTtlModal.vue'
import KvDetailModal from './KvDetailModal.vue'

const props = defineProps<{
  projectId: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  loading: [value: boolean]
}>()

const { t } = useI18n()

const rows = ref<KvKeyMeta[]>([])
const tab = ref<'data' | 'api'>('data')
const cursor = ref('')
const hasMore = ref(false)
const pattern = ref('')
const typeFilter = ref('all')
const loadingMore = ref(false)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run } = useLoadState({
  fallback: 'components.kvPanel.loadFailed',
  // 系统项目没有 KV catalog、只读实例无法补建：属于「空」，不是加载失败。
  emptyOn: isEmptyResourceError
})
watch(pending, (value) => emit('loading', value), { immediate: true })

const createOpen = ref(false)
const detailOpen = ref(false)
const detailTarget = ref<KvKeyMeta | null>(null)
const ttlOpen = ref(false)
const ttlTarget = ref<KvKeyMeta | null>(null)
const renameOpen = ref(false)
const renameTarget = ref<KvKeyMeta | null>(null)
const renameValue = ref('')
const renaming = ref(false)

// TTL 展示：ttl_ms 为拉取时刻的剩余值，结合拉取时间戳做本地倒计时
const nowTick = ref(Date.now())
const fetchedAt = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null
function ensureTimer() {
  const need = rows.value.some((m) => m.ttl_ms != null)
  if (need && !timer) {
    timer = setInterval(() => {
      nowTick.value = Date.now()
    }, 1000)
  } else if (!need && timer) {
    clearInterval(timer)
    timer = null
  }
}
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

function ttlText(m: KvKeyMeta): string {
  void nowTick.value // 建立响应式依赖
  if (m.ttl_ms == null) return t('components.kvPanel.ttlForever')
  const remain = m.ttl_ms - (Date.now() - fetchedAt.value)
  if (remain <= 0) return t('components.kvPanel.ttlExpiring')
  const sec = Math.ceil(remain / 1000)
  if (sec < 60) return t('components.kvPanel.ttlSeconds', { n: sec })
  if (sec < 3600) return t('components.kvPanel.ttlMinutes', { n: Math.floor(sec / 60) })
  return t('components.kvPanel.ttlHours', { n: Math.floor(sec / 3600) })
}

/** 类型对应的长度命令（读元数据的 len 列） */
function lenCommand(type: string, key: string): string[] | null {
  switch (type) {
    case 'string':
      return null
    case 'hash':
      return ['HLEN', key]
    case 'list':
      return ['LLEN', key]
    case 'set':
      return ['SCARD', key]
    case 'zset':
      return ['ZCARD', key]
    default:
      return null
  }
}

/** SCAN 拿 key 列表，再批量取 TYPE / PTTL / 长度拼出列表行。抛错前不改已有行。 */
async function scanPage(append: boolean): Promise<KvKeyMeta[]> {
  const argv = ['SCAN', append && cursor.value ? cursor.value : '0', 'COUNT', '100']
  if (pattern.value.trim()) argv.push('MATCH', pattern.value.trim())
  if (typeFilter.value !== 'all') argv.push('TYPE', typeFilter.value)
  const reply = (await api.kv.exec(props.projectId, { type: 'cmd', argvs: argv })) as [
    string,
    string[]
  ]
  const nextCursor = Array.isArray(reply) ? reply[0] : '0'
  const keys = Array.isArray(reply) ? reply[1] || [] : []
  const page = await Promise.all(
    keys.map(async (key) => {
      const metas = (await api.kv.execBatch(props.projectId, [
        { type: 'cmd', argvs: ['TYPE', key] },
        { type: 'cmd', argvs: ['PTTL', key] }
      ])) as [string, number]
      const type = metas[0] || 'string'
      const pttl = typeof metas[1] === 'number' ? metas[1] : -1
      let len: number | null = null
      if (type !== 'string') {
        const lc = lenCommand(type, key)
        if (lc) {
          const l = await api.kv.exec(props.projectId, { type: 'cmd', argvs: lc })
          len = typeof l === 'number' ? l : null
        }
      }
      return {
        key,
        type: type as KvKeyMeta['type'],
        len,
        ttl_ms: pttl >= 0 ? pttl : null,
        mtime_ms: 0,
        version: 0
      } satisfies KvKeyMeta
    })
  )
  rows.value = append ? rows.value.concat(page) : page
  cursor.value = nextCursor === '0' ? '' : nextCursor
  hasMore.value = cursor.value !== ''
  fetchedAt.value = Date.now()
  ensureTimer()
  return page
}

function load() {
  cursor.value = ''
  return run(async () => {
    const page = await scanPage(false)
    return page.length > 0
  })
}

function reload() {
  return load()
}

function openCreate() {
  if (props.readonly) return
  createOpen.value = true
}

defineExpose({ reload, openCreate })

async function loadMore() {
  loadingMore.value = true
  try {
    await scanPage(true)
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvPanel.loadFailed')))
  } finally {
    loadingMore.value = false
  }
}

function openDetail(m: KvKeyMeta) {
  detailTarget.value = m
  detailOpen.value = true
}

function openTtl(m: KvKeyMeta) {
  ttlTarget.value = m
  ttlOpen.value = true
}

function openRename(m: KvKeyMeta) {
  renameTarget.value = m
  renameValue.value = m.key
  renameOpen.value = true
}

async function submitRename() {
  const target = renameTarget.value
  const next = renameValue.value.trim()
  if (!target || !next || next === target.key) {
    renameOpen.value = false
    return
  }
  renaming.value = true
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['RENAME', target.key, next] })
    toast.success(t('components.kvPanel.renamed'))
    renameOpen.value = false
    reload()
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvPanel.renameFailed')))
  } finally {
    renaming.value = false
  }
}

async function removeKey(m: KvKeyMeta) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['DEL', m.key] })
    toast.success(t('components.kvPanel.deleted'))
    rows.value = rows.value.filter((r) => r.key !== m.key)
  } catch (e) {
    toast.error(errorMessage(e, t('components.kvPanel.deleteFailed')))
  }
}

function onCreated(key: string) {
  pattern.value = ''
  typeFilter.value = 'all'
  reload()
  void key
}

function onDeleted() {
  detailOpen.value = false
  reload()
}

// 挂载与切换项目时加载（面板跟随左侧导航的项目上下文）
watch(
  () => props.projectId,
  () => reload(),
  { immediate: true }
)
</script>
