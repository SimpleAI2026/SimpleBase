<template>
  <div class="flex flex-col gap-3 py-1">
    <Alert>
      <AlertTitle>项目级单端点</AlertTitle>
      <AlertDescription>
        所有命令都发到同一条路由：POST /v1/projects/:projectId/kv，body 为
        {"type":"cmd","argvs":[...]}。与「数据」页签展示的是同一份 kv.* 表数据。
      </AlertDescription>
    </Alert>

    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs text-muted-foreground">Base</span>
      <code class="sb-mono max-w-full truncate rounded bg-muted px-2 py-1 text-xs">{{ origin + base }}</code>
      <Button variant="outline" size="sm" @click="copy(baseFull, '已复制 Base 路径')">复制</Button>
    </div>
    <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>认证</span>
      <code class="rounded bg-muted px-2 py-1">Authorization: Bearer &lt;API_KEY&gt;</code>
      <span>（读命令要 database:read，写命令要 database:write）</span>
    </div>

    <Tabs v-model="group">
      <TabsList>
        <TabsTrigger v-for="g in KV_COMMAND_GROUPS" :key="g.value" class="px-3" :value="g.value">
          {{ g.label }}
        </TabsTrigger>
      </TabsList>
      <TabsContent v-for="g in KV_COMMAND_GROUPS" :key="'c-' + g.value" :value="g.value">
        <div class="overflow-x-auto rounded-lg border border-border/70 bg-card/60">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="w-40">命令</TableHead>
                <TableHead class="min-w-56 text-left">参数</TableHead>
                <TableHead class="text-left">语义</TableHead>
                <TableHead class="w-40">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="cmd in groupCommands" :key="cmd.name">
                <TableCell>
                  <Badge :variant="cmd.write ? 'default' : 'outline'">{{ cmd.name }}</Badge>
                </TableCell>
                <TableCell class="sb-mono max-w-96 truncate text-left text-xs" :title="cmd.args">
                  {{ cmd.args || '—' }}
                </TableCell>
                <TableCell class="text-left text-xs text-muted-foreground">{{ cmd.desc }}</TableCell>
                <TableCell>
                  <div class="flex items-center justify-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      :disabled="readonly && cmd.write"
                      :title="readonly && cmd.write ? '当前实例只读' : ''"
                      @click="copy(curl(cmd), '已复制 curl')"
                    >
                      复制 curl
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      :disabled="readonly && cmd.write"
                      @click="openTry(cmd)"
                    >
                      试调用
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </TabsContent>
    </Tabs>

    <SbModal
      :open="tryOpen"
      :title="tryTarget ? `试调用 ${tryTarget.name}` : ''"
      :width="560"
      :confirm-loading="trying"
      ok-text="执行"
      @ok="runTry"
      @update:open="tryOpen = $event"
    >
      <div v-if="tryTarget" class="flex flex-col gap-3">
        <div class="sb-mono max-w-full truncate rounded bg-muted px-2 py-1 text-xs">
          {{ tryTarget.name }} {{ tryTarget.args }}
        </div>
        <div v-if="usesKey">
          <FieldGroup>
            <Field>
              <FieldLabel for="kv-try-key">Key</FieldLabel>
              <Input id="kv-try-key" v-model="tryKey" placeholder="如 session:1001" />
            </Field>
          </FieldGroup>
        </div>
        <div v-if="usesValue">
          <FieldGroup>
            <Field>
              <FieldLabel for="kv-try-value">Value / Elem</FieldLabel>
              <Textarea id="kv-try-value" v-model="tryValue" rows="3" placeholder="文本值" />
            </Field>
          </FieldGroup>
        </div>
        <div class="rounded border border-border/70 bg-muted/40 p-2">
          <div class="mb-1 text-xs font-medium text-muted-foreground">请求体（自动生成）</div>
          <pre class="sb-mono max-h-56 overflow-auto text-xs">{{ tryBodyPreview }}</pre>
        </div>
        <div v-if="tryResult !== null" class="rounded border border-border/70 bg-muted/40 p-2">
          <div class="mb-1 text-xs font-medium text-muted-foreground">响应</div>
          <pre class="sb-mono max-h-56 overflow-auto text-xs">{{ tryResult }}</pre>
        </div>
      </div>
    </SbModal>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { api } from '../../../services/api'
import SbModal from '../../modal/SbModal.vue'
import { KV_COMMANDS, KV_COMMAND_GROUPS, kvBasePath, kvCurlSnippet, type KvCommand } from './kv-endpoints'

const props = defineProps<{
  projectId: string
  readonly?: boolean
}>()

const emit = defineEmits<{ changed: [] }>()

const group = ref<KvCommandGroup>('key')
const groupCommands = computed(() => KV_COMMANDS.filter((c) => c.group === group.value))

const base = computed(() => kvBasePath(props.projectId))
const baseFull = computed(() => window.location.origin + base.value)
const origin = typeof window !== 'undefined' ? window.location.origin : ''

function curl(cmd: KvCommand): string {
  return kvCurlSnippet(cmd, baseFull.value)
}

async function copy(text: string, message: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    } catch {
      // clipboard 全不可用（如测试环境）：静默降级，仍给反馈
    }
  }
  toast.success(message)
}

// —— 试调用 ——
const tryOpen = ref(false)
const tryTarget = ref<KvCommand | null>(null)
const tryKey = ref('')
const tryValue = ref('')
const trying = ref(false)
const tryResult = ref<string | null>(null)

// argv 里是否含 {key} / {value} 占位（决定表单展示哪些输入）
const usesKey = computed(() => !!tryTarget.value?.argv.includes('{key}'))
const usesValue = computed(() => !!tryTarget.value?.argv.includes('{value}'))

/** 请求体预览：占位符替换后的完整 JSON */
const tryBodyPreview = computed(() => {
  const cmd = tryTarget.value
  if (!cmd) return ''
  const argvs = cmd.argv.map((a) =>
    a === '{key}' ? tryKey.value.trim() || '{key}' : a === '{value}' ? tryValue.value || '{value}' : a
  )
  return JSON.stringify({ type: 'cmd', argvs }, null, 2)
})

function openTry(cmd: KvCommand) {
  tryTarget.value = cmd
  tryKey.value = 'session:1001'
  tryValue.value = 'hello'
  tryResult.value = null
  tryOpen.value = true
}

/** 试调用与「数据」页签走同一个项目级单端点客户端 */
async function runTry() {
  const ep = tryTarget.value
  if (!ep || trying.value) return
  trying.value = true
  tryResult.value = null
  try {
    const res = await api.kv.exec(props.projectId, {
      type: 'cmd',
      argvs: ep.argv.map((a) =>
        a === '{key}' ? tryKey.value.trim() : a === '{value}' ? tryValue.value : a
      )
    })
    tryResult.value = JSON.stringify(res ?? null, null, 2)
    if (ep.write) emit('changed') // 写成功后让「数据」页签刷新
  } catch (e) {
    tryResult.value = '错误：' + (e instanceof Error ? e.message : String(e))
  } finally {
    trying.value = false
  }
}
</script>
