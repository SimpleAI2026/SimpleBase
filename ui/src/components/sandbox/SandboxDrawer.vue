<template>
  <SbModal :open="open" :title="sandbox?.name || t('components.sandboxDrawer.fallbackTitle')" :max-width="900" hide-footer @update:open="$emit('update:open', $event)">
    <div v-if="sandbox" class="flex min-h-80 min-w-0 max-w-full flex-col gap-3 py-2">
      <SbTabsPanel v-model="tab" :items="tabItems" variant="line" item-content-class="min-w-0 pt-3">
        <template #terminal>
          <SandboxTerminal :project-id="projectId" :sandbox="sandbox" :capabilities="capabilities" :active="tab === 'terminal'" />
        </template>
        <template #files>
          <div class="flex min-w-0 flex-col gap-3">
            <div class="flex min-w-0 items-center gap-2 text-sm">
              <span class="sb-mono min-w-0 flex-1 truncate" :title="directory">{{ directory }}</span>
              <Button size="sm" variant="outline" class="shrink-0" @click="loadFiles">{{ t('components.sandboxDrawer.refresh') }}</Button>
            </div>
            <div class="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" :disabled="directory === ROOT_DIR" @click="goUp">{{ t('components.sandboxDrawer.goUp') }}</Button>
              <Button v-if="writable" size="sm" variant="outline" @click="createFile">{{ t('components.sandboxDrawer.newFile') }}</Button>
              <label v-if="writable" class="inline-flex cursor-pointer items-center rounded-md border px-3 text-sm">
                {{ t('components.sandboxDrawer.uploadFile') }}
                <input type="file" class="sr-only" @change="uploadFile" />
              </label>
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
              <div v-if="entries.length" class="min-w-0 divide-y rounded-md border">
                <div v-for="entry in entries" :key="entry.path" class="flex min-w-0 items-center justify-between gap-2 px-3 py-2 text-sm">
                  <button class="sb-mono min-w-0 flex-1 truncate text-left hover:underline" :title="entry.name" @click="openEntry(entry)">
                    {{ entry.kind === 'directory' ? t('components.sandboxDrawer.directoryTag') : '' }}{{ entry.name }}
                  </button>
                  <ConfirmAction
                    v-if="entry.kind !== 'directory' && writable"
                    :title="t('components.sandboxDrawer.deleteFileConfirm', { name: entry.name })"
                    @confirm="removeFile(entry.path)"
                  >
                    <Button class="shrink-0" variant="destructiveGhost" size="sm">{{ t('components.sandboxDrawer.delete') }}</Button>
                  </ConfirmAction>
                </div>
              </div>
            </SbAsyncRegion>
            <div v-if="filePath" class="min-w-0 space-y-2 rounded-md border p-3">
              <span class="sb-mono block break-all text-xs">{{ filePath }}</span>
              <SbBlockSkeleton v-if="reading" variant="lines" />
              <template v-else>
                <Textarea
                  v-model="fileText"
                  rows="8"
                  :aria-label="t('components.sandboxDrawer.fileContentAria')"
                  :readonly="!writable || fileBinary"
                />
                <div class="flex flex-wrap gap-2">
                  <Button v-if="writable && !fileBinary" size="sm" :disabled="saving" @click="saveFile">
                    <Spinner v-if="saving" data-icon="inline-start" />
                    {{ t('components.sandboxDrawer.save') }}
                  </Button>
                  <Button size="sm" variant="outline" @click="downloadFile">{{ t('components.sandboxDrawer.download') }}</Button>
                </div>
                <p v-if="fileBinary" class="text-xs text-muted-foreground">{{ t('components.sandboxDrawer.binaryHint') }}</p>
              </template>
            </div>
          </div>
        </template>
        <template #info>
          <div class="min-w-0 space-y-3 text-sm">
            <dl class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoState') }}</dt>
              <dd>{{ sandbox.state }} · {{ sandbox.source }}</dd>
              <dt class="text-muted-foreground">ID</dt>
              <dd class="sb-mono break-all">{{ sandbox.id }}</dd>
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoTemplate') }}</dt>
              <dd class="sb-mono break-all">{{ sandbox.templateId }}</dd>
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoSpec') }}</dt>
              <dd>{{ sandbox.cpuCount }}C / {{ sandbox.memoryMB }}MB</dd>
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoStarted') }}</dt>
              <dd>{{ sandbox.startedAt ? formatTime(sandbox.startedAt) : t('common.notStarted') }}</dd>
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoLastActive') }}</dt>
              <dd>{{ sandbox.lastActiveAt ? formatTime(sandbox.lastActiveAt) : t('common.notStarted') }}</dd>
              <dt class="text-muted-foreground">{{ t('components.sandboxDrawer.infoEndAt') }}</dt>
              <dd>{{ sandbox.endAt ? formatTime(sandbox.endAt) : '—' }}</dd>
            </dl>
            <p v-if="sandbox.lastError" class="break-all text-destructive">{{ sandbox.lastError }}</p>
            <section v-if="sshCommand" class="min-w-0 space-y-1">
              <h4 class="font-medium">{{ t('components.sandboxDrawer.sshTitle') }}</h4>
              <pre class="sb-mono overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-muted p-2 text-xs">{{ sshCommand }}</pre>
              <p v-if="capabilities?.ssh?.hostKeyFingerprint" class="sb-mono break-all text-xs text-muted-foreground">{{ capabilities.ssh.hostKeyFingerprint }}</p>
              <Button size="sm" variant="outline" @click="copyText(sshCommand, 'sshCopied')">{{ t('components.sandboxDrawer.copySsh') }}</Button>
            </section>
            <section class="min-w-0 space-y-1">
              <h4 class="font-medium">{{ t('components.sandboxDrawer.e2bTitle') }}</h4>
              <pre class="sb-mono overflow-x-auto whitespace-pre-wrap break-all rounded-md bg-muted p-2 text-xs">{{ e2bSnippet }}</pre>
              <div class="flex flex-wrap gap-2">
                <Button size="sm" variant="outline" @click="copyText(e2bSnippet, 'e2bCopied')">{{ t('components.sandboxDrawer.copyE2b') }}</Button>
                <Button size="sm" variant="outline" @click="copyText(curlExample, 'curlCopied')">{{ t('components.sandboxDrawer.copyCurlExample') }}</Button>
              </div>
            </section>
          </div>
        </template>
      </SbTabsPanel>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { useLoadState } from '@/composables/useLoadState'
import { errorMessage, formatTime } from '@/utils/format'
import type { SandboxCapabilities, SandboxFileEntry, SandboxItem } from '../../services/types'
import { api } from '../../services/api'
import { useAuthStore } from '../../stores/auth'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbBlockSkeleton from '@/components/SbBlockSkeleton.vue'
import SbTabsPanel from '@/components/SbTabsPanel.vue'
import SbModal from '../modal/SbModal.vue'
import ConfirmAction from '../ConfirmAction.vue'
import SandboxTerminal from './SandboxTerminal.vue'

const props = defineProps<{
  open: boolean
  projectId: string
  sandbox: SandboxItem | null
  capabilities: SandboxCapabilities | null
}>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const { t } = useI18n()
const auth = useAuthStore()

type TabKey = 'terminal' | 'files' | 'info'
const ROOT_DIR = '/workspace'

/** 页签：value 是语言无关的稳定标识，label 随语言变化。 */
const tabItems = computed(() => [
  { value: 'terminal', label: t('components.sandboxDrawer.tabTerminal') },
  { value: 'files', label: t('components.sandboxDrawer.tabFiles') },
  { value: 'info', label: t('components.sandboxDrawer.tabInfo') }
])
const tab = ref<TabKey>('terminal')
const saving = ref(false)
const directory = ref(ROOT_DIR)
const entries = ref<SandboxFileEntry[]>([])
const filePath = ref('')
const fileText = ref('')
const fileBinary = ref(false)
const reading = ref(false)
const { pending, showSkeleton, showEmpty, showError, refreshing, error, run, reset } = useLoadState({
  fallback: 'components.sandboxDrawer.loadFilesFailed'
})

/** 可写：有写权限，并且不是 agent 占用的沙盒。 */
const writable = computed(() => auth.canWrite && props.sandbox?.source !== 'agent')
const maxFileBytes = computed(() => props.capabilities?.maxFileBytes || 1048576)

watch(() => props.sandbox?.id, () => {
  tab.value = 'terminal'
  directory.value = ROOT_DIR
  entries.value = []
  filePath.value = ''
  reading.value = false
  reset()
})
watch(tab, (value) => { if (value === 'files' && props.open) void loadFiles() })

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
function goUp() {
  directory.value = directory.value.slice(0, directory.value.lastIndexOf('/')) || ROOT_DIR
  filePath.value = ''
  void loadFiles()
}
async function openEntry(entry: SandboxFileEntry) {
  if (!props.sandbox) return
  if (entry.kind === 'directory') {
    directory.value = entry.path
    filePath.value = ''
    await loadFiles()
    return
  }
  filePath.value = entry.path
  reading.value = true
  try {
    const blob = await api.sandboxes.files.read(props.projectId, props.sandbox.id, entry.path)
    if (blob.size > maxFileBytes.value) {
      fileBinary.value = true
      fileText.value = ''
      toast.warning(t('components.sandboxDrawer.contentTruncated'))
      return
    }
    try {
      fileText.value = new TextDecoder('utf-8', { fatal: true }).decode(await blob.arrayBuffer())
      fileBinary.value = false
    } catch {
      fileText.value = ''
      fileBinary.value = true
    }
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.readFailed'))) }
  finally { reading.value = false }
}
function createFile() { filePath.value = directory.value + '/new.txt'; fileText.value = ''; fileBinary.value = false }
async function saveFile() {
  if (!props.sandbox || saving.value) return
  saving.value = true
  try {
    await api.sandboxes.files.write(props.projectId, props.sandbox.id, filePath.value, fileText.value)
    await loadFiles()
    toast.success(t('components.sandboxDrawer.fileSaved'))
    emit('changed')
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.saveFailed'))) }
  finally { saving.value = false }
}
async function removeFile(path: string) {
  if (!props.sandbox) return
  try {
    await api.sandboxes.files.remove(props.projectId, props.sandbox.id, path)
    if (filePath.value === path) filePath.value = ''
    await loadFiles()
    toast.success(t('components.sandboxDrawer.fileDeleted'))
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.deleteFailed'))) }
}
async function uploadFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!props.sandbox || !file) return
  if (file.size > maxFileBytes.value) { toast.error(t('components.sandboxDrawer.fileTooLarge')); input.value = ''; return }
  try {
    await api.sandboxes.files.write(props.projectId, props.sandbox.id, directory.value + '/' + file.name, file)
    await loadFiles()
    toast.success(t('components.sandboxDrawer.uploadSuccess'))
    emit('changed')
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.uploadFailed'))) }
  finally { input.value = '' }
}
async function downloadFile() {
  if (!props.sandbox || !filePath.value) return
  try {
    const blob = await api.sandboxes.files.read(props.projectId, props.sandbox.id, filePath.value)
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = filePath.value.split('/').at(-1) || 'file'
    link.click()
    URL.revokeObjectURL(link.href)
  } catch (e) { toast.error(errorMessage(e, t('components.sandboxDrawer.downloadFailed'))) }
}

/** SSH 登录命令：仅当服务端开启了 SSH 监听。 */
const sshCommand = computed(() => {
  const ssh = props.capabilities?.ssh
  if (!ssh?.enabled || !ssh.listen || !props.sandbox) return ''
  const port = ssh.listen.slice(ssh.listen.lastIndexOf(':') + 1)
  return `ssh -p ${port} ${props.sandbox.id}@${window.location.hostname}`
})
const e2bSnippet = computed(() => {
  const origin = window.location.origin
  return [
    'export E2B_API_KEY=<your SimpleBase API key>',
    `export E2B_API_URL=${origin}/e2b`,
    `export E2B_SANDBOX_URL=${origin}`,
    '',
    "import { Sandbox } from 'e2b'",
    `const sbx = await Sandbox.connect('${props.sandbox?.id ?? ''}')`
  ].join('\n')
})
const curlExample = computed(
  () => `curl '${window.location.origin}/e2b/v2/sandboxes' -H 'X-API-Key: $API_KEY' -H 'X-SimpleBase-Project: ${props.projectId}'`
)

async function copyText(text: string, toastKey: 'sshCopied' | 'e2bCopied' | 'curlCopied') {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('components.sandboxDrawer.' + toastKey))
  } catch { toast.error(t('components.sandboxDrawer.clipboardFailed')) }
}
</script>
