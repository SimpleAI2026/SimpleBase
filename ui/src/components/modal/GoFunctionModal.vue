<template>
  <SbModal
    :open="open"
    :title="title"
    :width="960"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex flex-col gap-4">
      <div class="flex items-center gap-2">
        <Label class="shrink-0 text-sm">云函数名</Label>
        <Input
          v-model="nameValue"
          class="w-56 sb-mono"
          :disabled="mode !== 'create'"
          placeholder="如 hello"
        />
        <span class="sb-mono text-sm text-muted-foreground">.go</span>
      </div>

      <!-- 固定 HTTP+JSON 约定提示（不可关闭） -->
      <Alert>
        <TerminalIcon class="size-4" />
        <AlertTitle>HTTP+JSON 约定</AlertTitle>
        <AlertDescription class="space-y-1">
          <p>每个导出函数必须是 <code class="sb-mono">func Name(req T) R</code>（1 个入参、1 个返回值）。</p>
          <p>
            调用：<code class="sb-mono">POST /go/{{ projectId }}/{{ nameValue || '{name}' }}/{Name}</code>，
            Content-Type: application/json。Body 映射到 req，响应体是 R 的 JSON。
          </p>
          <p>请使用大写函数名，否则不会出现在列表、也无法调用。入参为基本类型时，body 用对应 JSON 字面量（如 <code class="sb-mono">"abc"</code>）；入参为 struct 时用 JSON 对象。</p>
        </AlertDescription>
      </Alert>

      <div v-if="previewExports.length" class="flex flex-wrap items-center gap-1.5">
        <span class="text-xs text-muted-foreground">将导出</span>
        <Badge v-for="fn in previewExports" :key="fn" variant="secondary" class="sb-mono">
          {{ fn }}
        </Badge>
        <span class="text-xs text-muted-foreground">← 前端正则预览；保存以服务端校验为准</span>
      </div>

      <div :style="{ minHeight: editorMinHeight + 'px' }" class="min-w-0">
        <GoMonacoEditor v-model="sourceValue" :read-only="mode === 'view'" :min-height="editorMinHeight" />
      </div>
    </div>
    <template #footer>
      <template v-if="mode === 'view'">
        <Button variant="outline" @click="close">关闭</Button>
      </template>
      <template v-else>
        <Button variant="outline" :disabled="saving" @click="close">取消</Button>
        <Button :disabled="saveDisabled || saving" @click="save">
          <Spinner v-if="saving" data-icon="inline-start" />
          保存
        </Button>
      </template>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 云函数统一弹窗：create / edit / view 三模式共用（ui-gofunction-plan §8.4）。
 * 导出预览用正则即时渲染（不当校验）；保存失败 toast 展示后端编译错误原文。
 */
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { TerminalIcon } from '@lucide/vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import SbModal from './SbModal.vue'
import GoMonacoEditor from '@/components/editor/GoMonacoEditor.vue'
import { api } from '../../services/api'
import { useProjectStore } from '../../stores/project'

const props = withDefaults(
  defineProps<{
    open: boolean
    mode: 'create' | 'edit' | 'view'
    /** edit/view 模式下的目标云函数 */
    target?: { name: string; source?: string }
  }>(),
  { target: undefined }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const projectStore = useProjectStore()
const projectId = computed(() => projectStore.projectId)

const editorMinHeight = 420
const defaultTemplate = [
  'package main',
  '',
  'type Request struct {',
  '\tName string `json:"name"`',
  '}',
  '',
  'type Response struct {',
  '\tMessage string `json:"message"`',
  '}',
  '',
  'func Hello(req Request) Response {',
  '\treturn Response{Message: "hello, " + req.Name}',
  '}'
].join('\n')

const nameValue = ref('')
const sourceValue = ref(defaultTemplate)
const saving = ref(false)

const title = computed(() => {
  if (props.mode === 'create') return '新建云函数'
  const n = props.target?.name ?? nameValue.value
  return `${props.mode === 'edit' ? '编辑' : '查看'} ${n}.go`
})

const saveDisabled = computed(() => {
  if (props.mode !== 'create') return false
  return !nameValue.value.trim() || !sourceValue.value.trim()
})

// 前端正则预览（非校验）
const previewExports = computed(() => {
  const names = [...sourceValue.value.matchAll(/^func\s+([A-Z][A-Za-z0-9_]*)\s*\(/gm)].map((m) => m[1])
  return [...new Set(names)]
})

watch(
  () => props.open,
  (open) => {
    if (!open) return
    if (props.mode === 'create') {
      nameValue.value = ''
      sourceValue.value = defaultTemplate
    } else if (props.target) {
      nameValue.value = props.target.name
      if (props.target.source != null) {
        sourceValue.value = props.target.source
      } else {
        // 列表不带 source：打开时按需拉详情
        api.gofunctions
          .get(projectId.value, props.target.name)
          .then((g) => {
            if (g.source != null) sourceValue.value = g.source
          })
          .catch(() => toast.error('加载云函数源码失败'))
      }
    }
  }
)

function close() {
  emit('update:open', false)
}

async function save() {
  if (saving.value) return
  saving.value = true
  try {
    if (props.mode === 'create') {
      const created = await api.gofunctions.create(projectId.value, {
        name: nameValue.value.trim(),
        source: sourceValue.value
      })
      toast.success(`已创建 ${created.file}`)
    } else if (props.target) {
      const updated = await api.gofunctions.update(projectId.value, props.target.name, sourceValue.value)
      toast.success(`已保存 ${updated.file}，导出：${updated.exports.join('、') || '无'}`)
    }
    emit('saved')
    close()
  } catch (e) {
    // 后端编译/签名错误原文透传（G7）
    toast.error(e instanceof Error ? e.message : '保存失败')
  } finally {
    saving.value = false
  }
}
</script>
