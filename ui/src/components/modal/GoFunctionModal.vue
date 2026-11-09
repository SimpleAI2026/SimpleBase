<template>
  <SbModal
    :open="open"
    :title="title"
    :width="960"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex flex-col gap-4">
      <div class="flex flex-wrap items-center gap-2">
        <Label class="shrink-0 text-sm">{{ t('components.goFunctionModal.nameLabel') }}</Label>
        <Input
          v-model="nameValue"
          class="sb-mono min-w-0 flex-1 sm:w-56 sm:flex-none"
          :disabled="mode !== 'create'"
          :placeholder="t('components.goFunctionModal.namePlaceholder')"
        />
        <span class="sb-mono text-sm text-muted-foreground">.go</span>
      </div>

      <!-- 固定 HTTP+JSON 约定提示（不可关闭） -->
      <Alert>
        <TerminalIcon class="size-4" />
        <AlertTitle>{{ t('components.goFunctionModal.protocolTitle') }}</AlertTitle>
        <AlertDescription class="space-y-1">
          <p>{{ t('components.goFunctionModal.signatureHint') }}</p>
          <p>
            {{ t('components.goFunctionModal.callHint') }}
            <code class="sb-code-view sb-mono max-w-full min-w-0 whitespace-pre-wrap [word-break:break-word] [overflow-wrap:anywhere]">POST /go/{{ projectId }}/{{ nameValue || 'name' }}/{Name}</code>
          </p>
          <p>{{ t('components.goFunctionModal.uppercaseHint') }}</p>
        </AlertDescription>
      </Alert>

      <div v-if="previewExports.length" class="flex flex-wrap items-center gap-1.5">
        <span class="text-xs text-muted-foreground">{{ t('components.goFunctionModal.willExport') }}</span>
        <Badge v-for="fn in previewExports" :key="fn" variant="secondary" class="sb-mono">
          {{ fn }}
        </Badge>
        <span class="text-xs text-muted-foreground">{{ t('components.goFunctionModal.regexPreview') }}</span>
      </div>

      <div :style="{ minHeight: editorMinHeight + 'px' }" class="min-w-0 max-w-full overflow-hidden">
        <GoMonacoEditor v-model="sourceValue" :read-only="mode === 'view'" :min-height="editorMinHeight" />
      </div>
    </div>
    <template #footer>
      <template v-if="mode === 'view'">
        <Button variant="outline" @click="close">{{ t('common.close') }}</Button>
      </template>
      <template v-else>
        <Button variant="outline" :disabled="saving" @click="close">{{ t('common.cancel') }}</Button>
        <Button :disabled="saveDisabled || saving" @click="save">
          <Spinner v-if="saving" data-icon="inline-start" />
          {{ mode === 'create' ? t('components.goFunctionModal.create') : t('components.goFunctionModal.saveAsNewVersion') }}
        </Button>
      </template>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
import { errorMessage } from '@/utils/format'
/**
 * 云函数统一弹窗：create / edit / view 三模式共用（ui-gofunction-plan §8.4）。
 * 导出预览用正则即时渲染（不当校验）；保存失败 toast 展示后端编译错误原文。
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()
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
  if (props.mode === 'create') return t('components.goFunctionModal.titleCreate')
  const n = props.target?.name ?? nameValue.value
  return props.mode === 'edit'
    ? t('components.goFunctionModal.titleEdit', { name: n })
    : t('components.goFunctionModal.titleView', { name: n })
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
          .catch(() => toast.error(t('components.goFunctionModal.loadSourceFailed')))
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
        source: sourceValue.value,
        activate: true
      })
      toast.success(t('components.goFunctionModal.created', { file: created.file, n: created.activeVersion || 1 }))
    } else if (props.target) {
      const updated = await api.gofunctions.saveVersion(projectId.value, props.target.name, {
        source: sourceValue.value,
        activate: true
      })
      toast.success(t('components.goFunctionModal.savedVersion', { n: updated.latestVersion || updated.activeVersion || '' }))
    }
    emit('saved')
    close()
  } catch (e) {
    // 后端编译/签名错误原文透传（G7）
    toast.error(errorMessage(e, t('components.goFunctionModal.saveFailed')))
  } finally {
    saving.value = false
  }
}
</script>
