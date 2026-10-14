<template>
  <SbModal :open="open" :title="sandbox?.name || t('components.sandboxDrawer.fallbackTitle')" :max-width="900" hide-footer @update:open="$emit('update:open', $event)">
    <div v-if="sandbox" class="flex max-h-[min(72vh,calc(100vh-12rem))] min-h-80 min-w-0 max-w-full flex-col gap-3 overflow-y-auto py-2">
      <div class="flex gap-2 border-b pb-2">
        <Button
          v-for="item in TABS"
          :key="item.value"
          size="sm"
          :variant="tab === item.value ? 'default' : 'ghost'"
          @click="tab = item.value"
        >
          {{ t(item.labelKey) }}
        </Button>
      </div>
      <template v-if="tab === 'terminal'">
        <p v-if="sandbox.source === 'agent'" class="text-sm text-muted-foreground">{{ t('components.sandboxDrawer.agentReadonly') }}</p>
        <div v-else class="flex gap-2">
          <Input
            v-model="command"
            :aria-label="t('components.sandboxDrawer.commandAria')"
            :placeholder="t('components.sandboxDrawer.commandPlaceholder')"
            :disabled="running"
            @keyup.enter="execute"
          />
          <Button :disabled="running || !command.trim()" @click="execute">
            <Spinner v-if="running" data-icon="inline-start" />
            {{ running ? t('components.sandboxDrawer.coldStart') : t('components.sandboxDrawer.execute') }}
          </Button>
        </div>
        <div v-if="history.length" class="space-y-3 rounded-lg border bg-muted/30 p-3">
          <div v-for="(entry, index) in history" :key="index" class="border-b pb-2 last:border-0">
            <div class="sb-mono text-xs text-muted-foreground">$ {{ entry.command }}</div>
            <pre v-if="entry.stdout" class="overflow-auto whitespace-pre-wrap break-all text-sm">{{ entry.stdout }}</pre>
            <pre v-if="entry.stderr" class="overflow-auto whitespace-pre-wrap break-all text-sm text-destructive">{{ entry.stderr }}</pre>
            <span class="text-xs text-muted-foreground">{{ t('components.sandboxDrawer.exitMeta', { code: entry.exitCode, duration: entry.durationMs }) }}{{ entry.timedOut ? t('components.sandboxDrawer.timedOut') : '' }}{{ entry.stdoutTruncated || entry.stderrTruncated ? t('components.sandboxDrawer.truncatedSuffix') : '' }}</span>
          </div>
        </div>
        <p v-else class="text-sm text-muted-foreground">{{ t('components.sandboxDrawer.noCommand') }}</p>
      </template>
      <template v-else-if="tab === 'files'">
        <div class="flex items-center gap-2 text-sm"><span class="sb-mono">{{ directory }}</span><Button size="sm" variant="outline" @click="loadFiles">{{ t('components.sandboxDrawer.refresh') }}</Button></div>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" :disabled="directory === '/workspace'" @click="directory = directory.slice(0, directory.lastIndexOf('/')) || '/workspace'; loadFiles()">{{ t('components.sandboxDrawer.goUp') }}</Button>
          <Button v-if="sandbox.source !== 'agent'" size="sm" variant="outline" @click="createFile">{{ t('components.sandboxDrawer.newFile') }}</Button>
          <label v-if="sandbox.source !== 'agent'" class="inline-flex cursor-pointer items-center rounded-md border px-3 text-sm">{{ t('components.sandboxDrawer.uploadFile') }}<input type="file" class="sr-only" @change="uploadFile" /></label>
        </div>
        <SbAsyncRegion
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          block="spinner"
          :empty-title="t('components.sandboxDrawer.emptyDirTitle')"
          :empty-description="t('components.sandboxDrawer.emptyDirDesc')"
          @retry="loadFiles"
        >
          <div v-if="entries.length" class="divide-y rounded-md border">
            <div v-for="entry in entries" :key="entry.path" class="flex items-center justify-between gap-2 px-3 py-2 text-sm">
              <button class="sb-mono truncate text-left hover:underline" @click="openEntry(entry)">{{ entry.kind === 'directory' ? t('components.sandboxDrawer.directoryTag') : '' }}{{ entry.name }}</button>
              <ConfirmAction
                v-if="entry.kind !== 'directory' && sandbox.source !== 'agent'"
                :title="t('components.sandboxDrawer.deleteFileConfirm', { name: entry.name })"
                @confirm="removeFile(entry.path)"
              ><Button variant="destructiveGhost" size="sm">{{ t('components.sandboxDrawer.delete') }}</Button></ConfirmAction>
            </div>
          </div>
        </SbAsyncRegion>
        <div v-if="filePath" class="space-y-2 rounded-md border p-3">
          <span class="sb-mono text-xs">{{ filePath }}</span>
          <SbBlockSkeleton v-if="reading" variant="lines" />
          <template v-else>
            <Textarea
              v-model="fileText"
              rows="8"
              :aria-label="t('components.sandboxDrawer.fileContentAria')"
              :readonly="sandbox.source === 'agent' || fileBinary"
            />
            <div class="flex gap-2">
              <Button v-if="sandbox.source !== 'agent' && !fileBinary" size="sm" :disabled="saving" @click="saveFile">
                <Spinner v-if="saving" data-icon="inline-start" />
                {{ t('components.sandboxDrawer.save') }}
              </Button>
              <Button size="sm" variant="outline" @click="downloadFile">{{ t('components.sandboxDrawer.download') }}</Button>
            </div>
            <p v-if="fileBinary" class="text-xs text-muted-foreground">{{ t('components.sandboxDrawer.binaryHint') }}</p>
          </template>
        </div>
      </template>
      <div v-else class="space-y-2 text-sm">
        <p>{{ t('components.sandboxDrawer.statusLabel', { status: sandbox.status, source: sandbox.source }) }}</p>
        <p class="sb-mono break-all">ID：{{ sandbox.id }}</p>
        <p class="sb-mono break-all">Cloud：{{ sandbox.cloudName }}</p>
        <p>{{ t('components.sandboxDrawer.specLabel', { image: sandbox.image, cpus: sandbox.cpus, memory: sandbox.memoryMiB, network: sandbox.network }) }}</p>
        <p>{{ t('components.sandboxDrawer.timeLabel', { created: sandbox.createdAt, active: sandbox.lastActiveAt || t('common.notStarted') }) }}</p>
        <p v-if="sandbox.lastError" class="text-destructive">{{ sandbox.lastError }}</p>
        <Button size="sm" variant="outline" @click="copyExample">{{ t('components.sandboxDrawer.copyCurlExample') }}</Button>
      </div>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { errorMessage } from '@/utils/format'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { useLoadState } from '@/composables/useLoadState'
import type { SandboxItem, SandboxExecResult, SandboxFileEntry } from '../../services/types'
import { api } from '../../services/api'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import SbModal from '../modal/SbModal.vue'
import ConfirmAction from '../ConfirmAction.vue'

const props = defineProps<{ open: boolean; projectId: string; sandbox: SandboxItem | null; maxFileBytes: number }>()
const { t } = useI18n()

type TabKey = 'terminal' | 'files' | 'info'

/** 页签：value 是语言无关的稳定标识，labelKey 才随语言变化。 */
const TABS: { value: TabKey; labelKey: string }[] = [
  { value: 'terminal', labelKey: 'components.sandboxDrawer.tabTerminal' },
  { value: 'files', labelKey: 'components.sandboxDrawer.tabFiles' },
  { value: 'info', labelKey: 'components.sandboxDrawer.tabInfo' }
]
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const tab = ref<TabKey>('terminal')
const command = ref('')
const running = ref(false)
const saving = ref(false)
const history = ref<(SandboxExecResult & { command: string })[]>([])
const directory = ref('/workspace')
const entries = ref<SandboxFileEntry[]>([])
const filePath = ref('')
const fileText = ref('')
const fileBinary = ref(false)
const reading = ref(false)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run, reset } = useLoadState({
  fallback: 'components.sandboxDrawer.loadFilesFailed'
})
watch(() => props.sandbox?.id, () => {
  tab.value = 'terminal'
  history.value = []
  directory.value = '/workspace'
  entries.value = []
  filePath.value = ''
  reading.value = false
  reset()
})
watch(tab, (value) => { if (value === 'files' && props.open) loadFiles() })
async function execute() {
  if (!props.sandbox || props.sandbox.source === 'agent' || !command.value.trim() || running.value) return
  const text = command.value.trim()
  running.value = true
  try {
    const result = await api.sandboxes.exec(props.projectId, props.sandbox.id, { command: text })
    history.value.unshift({ command: text, ...result })
    command.value = ''
    emit('changed')
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.executeFailed'))) }
  finally { running.value = false }
}
async function loadFiles() {
  if (!props.sandbox) return
  const sandbox = props.sandbox
  const dir = directory.value
  return run(async () => {
    const list = await api.sandboxes.files.list(props.projectId, sandbox.id, dir)
    entries.value = list
    return list.length > 0
  })
}
async function openEntry(entry: SandboxFileEntry) {
  if (!props.sandbox) return
  if (entry.kind === 'directory') { directory.value = entry.path; filePath.value = ''; await loadFiles(); return }
  filePath.value = entry.path
  reading.value = true
  try {
    const data = await api.sandboxes.files.read(props.projectId, props.sandbox.id, entry.path)
    fileBinary.value = data.encoding === 'base64'
    fileText.value = fileBinary.value ? '' : data.content
    if (data.truncated) toast.warning(t('components.sandboxDrawer.contentTruncated'))
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.readFailed'))) }
  finally { reading.value = false }
}
function createFile() { filePath.value = directory.value + '/new.txt'; fileText.value = ''; fileBinary.value = false }
async function saveFile() {
  if (!props.sandbox || saving.value) return
  saving.value = true
  try { await api.sandboxes.files.write(props.projectId, props.sandbox.id, filePath.value, fileText.value); await loadFiles(); toast.success(t('components.sandboxDrawer.fileSaved')); emit('changed') }
  catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.saveFailed'))) }
  finally { saving.value = false }
}
async function removeFile(path: string) {
  if (!props.sandbox) return
  try { await api.sandboxes.files.remove(props.projectId, props.sandbox.id, path); if (filePath.value === path) filePath.value = ''; await loadFiles(); toast.success(t('components.sandboxDrawer.fileDeleted')) }
  catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.deleteFailed'))) }
}
async function uploadFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!props.sandbox || !file) return
  if (file.size > props.maxFileBytes) { toast.error(t('components.sandboxDrawer.fileTooLarge')); return }
  try { await api.sandboxes.files.upload(props.projectId, props.sandbox.id, directory.value + '/' + file.name, new Uint8Array(await file.arrayBuffer())); await loadFiles(); toast.success(t('components.sandboxDrawer.uploadSuccess')) }
  catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.uploadFailed'))) }
  finally { input.value = '' }
}
async function downloadFile() {
  if (!props.sandbox || !filePath.value) return
  try {
    const blob = await api.sandboxes.files.download(props.projectId, props.sandbox.id, filePath.value)
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = filePath.value.split('/').at(-1) || 'file'
    link.click()
    URL.revokeObjectURL(link.href)
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.downloadFailed'))) }
}
async function copyExample() {
  if (!props.sandbox) return
  const example = `curl -X POST '$BASE_URL/v1/projects/${encodeURIComponent(props.projectId)}/sandboxes/${encodeURIComponent(props.sandbox.id)}/exec' -H 'Authorization: Bearer $API_KEY' -H 'Content-Type: application/json' -d '{"command":"echo hello"}'`
  try { await navigator.clipboard.writeText(example); toast.success(t('components.sandboxDrawer.curlCopied')) }
  catch { toast.error(t('components.sandboxDrawer.clipboardFailed')) }
}
</script>
