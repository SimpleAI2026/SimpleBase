<template>
  <div class="flex flex-col gap-3 py-1">
    <Tabs v-model="tab" class="gap-1">
      <TabsList>
        <TabsTrigger class="px-3" value="data">数据</TabsTrigger>
        <TabsTrigger class="px-3" value="api">API</TabsTrigger>
      </TabsList>
      <TabsContent value="data" class="mt-1 flex flex-col gap-3">
        <!-- 工具条：搜索 / 类型过滤。刷新与新建在页面标题右侧 -->
        <div class="flex flex-wrap items-center gap-2">
          <Input
            v-model="pattern"
            placeholder="按 key 匹配（支持 * ? [abc]）"
            class="w-64"
            clearable
            @keydown.enter="reload"
          />
          <Select v-model="typeFilter">
            <SelectTrigger class="w-32"><SelectValue placeholder="全部类型" /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部类型</SelectItem>
              <SelectItem value="string">string</SelectItem>
              <SelectItem value="hash">hash</SelectItem>
              <SelectItem value="list">list</SelectItem>
              <SelectItem value="set">set</SelectItem>
              <SelectItem value="zset">zset</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <Alert v-if="readonly">
          <AlertTitle>只读实例</AlertTitle>
          <AlertDescription>当前节点为只读，浏览数据不影响同步；写操作不可用。</AlertDescription>
        </Alert>

        <!-- 列表 -->
        <div class="overflow-hidden rounded-md border border-border/70">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-[38%]">Key</TableHead>
                <TableHead class="w-20">类型</TableHead>
                <TableHead class="w-24">长度</TableHead>
                <TableHead class="w-32">TTL</TableHead>
                <TableHead class="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <template v-if="loading && !rows.length">
                <TableRow v-for="i in 5" :key="i">
                  <TableCell colspan="5"><Skeleton class="h-5 w-full" /></TableCell>
                </TableRow>
              </template>
              <TableEmpty v-else-if="!rows.length" :colspan="5">
                <SbEmptyState
                  title="暂无 Key"
                  description="项目 Key-Value 会随项目自动准备，新建一个 Key 即可开始使用。"
                  :action-text="readonly ? undefined : '新建 Key'"
                  @action="openCreate"
                />
              </TableEmpty>
              <template v-else>
                <TableRow v-for="m in rows" :key="m.key" class="cursor-pointer" @click="openDetail(m)">
                  <TableCell class="sb-mono max-w-0 truncate">{{ m.key }}</TableCell>
                  <TableCell>
                    <Badge :variant="typeBadge(m.type)">{{ m.type }}</Badge>
                  </TableCell>
                  <TableCell class="sb-mono">{{ m.len ?? '—' }}</TableCell>
                  <TableCell class="text-muted-foreground">{{ ttlText(m) }}</TableCell>
                  <TableCell class="text-right">
                    <div class="inline-flex gap-1" @click.stop>
                      <Button size="sm" variant="ghost" :disabled="readonly" @click="openTtl(m)">TTL</Button>
                      <Button size="sm" variant="ghost" :disabled="readonly" @click="openRename(m)">重命名</Button>
                      <ConfirmAction
                        title="删除 Key"
                        :description="`删除 ${m.key}？此操作不可恢复。`"
                        :disabled="readonly"
                        @confirm="removeKey(m)"
                      >
                        <Button size="sm" variant="ghost" class="text-destructive hover:text-destructive">删除</Button>
                      </ConfirmAction>
                    </div>
                  </TableCell>
                </TableRow>
              </template>
            </TableBody>
          </Table>
        </div>

        <div v-if="hasMore" class="flex justify-center">
          <Button size="sm" variant="outline" :disabled="loading" @click="loadMore">
            <Spinner v-if="loading" class="mr-2 h-3.5 w-3.5" />
            加载更多
          </Button>
        </div>
      </TabsContent>

      <TabsContent value="api" class="mt-1">
        <KvApiPanel :project-id="projectId" :readonly="readonly" @changed="reload" />
      </TabsContent>
    </Tabs>

    <KvCreateKeyModal v-model:open="createOpen" :project-id="projectId" :readonly="readonly" @created="onCreated" />
    <KvTtlModal v-model:open="ttlOpen" :project-id="projectId" :kv-key="ttlTarget" @changed="reload" />
    <KvDetailSheet
      v-model:open="detailOpen"
      :project-id="projectId"
      :kv-key="detailTarget"
      :readonly="readonly"
      @changed="reload"
      @deleted="onDeleted"
    />

    <SbModal
      :open="renameOpen"
      title="重命名 Key"
      :width="440"
      :confirm-loading="renaming"
      :ok-button-props="{ disabled: !renameValue.trim() }"
      @ok="submitRename"
      @update:open="renameOpen = $event"
    >
      <FieldGroup>
        <Field>
          <FieldLabel for="kv-rename">新名称</FieldLabel>
          <Input id="kv-rename" v-model="renameValue" placeholder="新的 key 名称" @keydown.enter="submitRename" />
        </Field>
      </FieldGroup>
    </SbModal>
  </div>
</template>

<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableEmpty, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api } from '../../../services/api'
import type { KvKeyMeta } from '../../../services/api'
import SbEmptyState from '../../SbEmptyState.vue'
import ConfirmAction from '../../ConfirmAction.vue'
import KvApiPanel from './KvApiPanel.vue'
import SbModal from '../../modal/SbModal.vue'
import KvCreateKeyModal from './KvCreateKeyModal.vue'
import KvTtlModal from './KvTtlModal.vue'
import KvDetailSheet from './KvDetailSheet.vue'

const props = defineProps<{
  projectId: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  loading: [value: boolean]
}>()

const rows = ref<KvKeyMeta[]>([])
const tab = ref<'data' | 'api'>('data')
const cursor = ref('')
const hasMore = ref(false)
const pattern = ref('')
const typeFilter = ref('all')
const loading = ref(false)
watch(loading, (value) => emit('loading', value), { immediate: true })

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

function typeBadge(t: string): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (t) {
    case 'hash':
      return 'secondary'
    case 'list':
      return 'outline'
    case 'set':
      return 'default'
    case 'zset':
      return 'destructive'
    default:
      return 'default'
  }
}

function ttlText(m: KvKeyMeta): string {
  void nowTick.value // 建立响应式依赖
  if (m.ttl_ms == null) return '永久'
  const remain = m.ttl_ms - (Date.now() - fetchedAt.value)
  if (remain <= 0) return '即将过期'
  const sec = Math.ceil(remain / 1000)
  if (sec < 60) return sec + 's 后过期'
  if (sec < 3600) return Math.floor(sec / 60) + 'm 后过期'
  return Math.floor(sec / 3600) + 'h 后过期'
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

/** SCAN 拿 key 列表，再批量取 TYPE / PTTL / 长度拼出列表行 */
async function fetchPage(append: boolean) {
  loading.value = true
  try {
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
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '加载失败')
  } finally {
    loading.value = false
  }
}

function reload() {
  cursor.value = ''
  void fetchPage(false)
}

function openCreate() {
  if (props.readonly) return
  createOpen.value = true
}

defineExpose({ reload, openCreate })

function loadMore() {
  void fetchPage(true)
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
    toast.success('已重命名')
    renameOpen.value = false
    reload()
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '重命名失败')
  } finally {
    renaming.value = false
  }
}

async function removeKey(m: KvKeyMeta) {
  try {
    await api.kv.exec(props.projectId, { type: 'cmd', argvs: ['DEL', m.key] })
    toast.success('已删除')
    rows.value = rows.value.filter((r) => r.key !== m.key)
  } catch (e) {
    toast.error(e instanceof Error ? e.message : '删除失败')
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
