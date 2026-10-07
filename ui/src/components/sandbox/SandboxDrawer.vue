<template>
  <SbModal :open="open" :title="sandbox?.name || '云沙盒'" :max-width="900" hide-footer @update:open="$emit('update:open', $event)">
    <div v-if="sandbox" class="flex max-h-[72vh] min-h-80 flex-col gap-3 overflow-y-auto py-2">
      <div class="flex gap-2 border-b pb-2">
        <Button v-for="name in ['终端', '文件', '信息']" :key="name" size="sm" :variant="tab === name ? 'default' : 'ghost'" @click="tab = name">{{ name }}</Button>
      </div>
      <template v-if="tab === '终端'">
        <p v-if="sandbox.source === 'agent'" class="text-sm text-muted-foreground">此沙盒由 Agent 会话使用，终端只读。</p>
        <div v-else class="flex gap-2">
          <Input v-model="command" aria-label="沙盒命令" placeholder="输入命令，例如 python -V" :disabled="running" @keyup.enter="execute" />
          <Button :disabled="running || !command.trim()" @click="execute">{{ running ? '冷启动中…' : '执行' }}</Button>
        </div>
        <div v-if="history.length" class="space-y-3 rounded-lg border bg-muted/30 p-3">
          <div v-for="(entry, index) in history" :key="index" class="border-b pb-2 last:border-0">
            <div class="sb-mono text-xs text-muted-foreground">$ {{ entry.command }}</div>
            <pre v-if="entry.stdout" class="overflow-auto whitespace-pre-wrap break-all text-sm">{{ entry.stdout }}</pre>
            <pre v-if="entry.stderr" class="overflow-auto whitespace-pre-wrap break-all text-sm text-destructive">{{ entry.stderr }}</pre>
            <span class="text-xs text-muted-foreground">exit {{ entry.exitCode }} · {{ entry.durationMs }}ms{{ entry.timedOut ? ' · 超时' : '' }}{{ entry.stdoutTruncated || entry.stderrTruncated ? ' · 输出已截断' : '' }}</span>
          </div>
        </div>
        <p v-else class="text-sm text-muted-foreground">尚未执行命令。执行是同步请求，输出会一次性返回。</p>
      </template>
      <template v-else-if="tab === '文件'">
        <div class="flex items-center gap-2 text-sm"><span class="sb-mono">{{ directory }}</span><Button size="sm" variant="outline" @click="loadFiles">刷新</Button></div>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" :disabled="directory === '/workspace'" @click="directory = directory.slice(0, directory.lastIndexOf('/')) || '/workspace'; loadFiles()">上一级</Button>
          <Button v-if="sandbox.source !== 'agent'" size="sm" variant="outline" @click="createFile">新建文件</Button>
          <label v-if="sandbox.source !== 'agent'" class="inline-flex cursor-pointer items-center rounded-md border px-3 text-sm">上传文件<input type="file" class="sr-only" @change="uploadFile" /></label>
        </div>
        <div v-if="entries.length" class="divide-y rounded-md border">
          <div v-for="entry in entries" :key="entry.path" class="flex items-center justify-between gap-2 px-3 py-2 text-sm">
            <button class="sb-mono truncate text-left hover:underline" @click="openEntry(entry)">{{ entry.kind === 'directory' ? '[目录] ' : '' }}{{ entry.name }}</button>
            <ConfirmAction v-if="entry.kind !== 'directory' && sandbox.source !== 'agent'" :title="`确认删除 ${entry.name}？`" @confirm="removeFile(entry.path)"><Button variant="destructiveGhost" size="sm">删除</Button></ConfirmAction>
          </div>
        </div>
        <SbEmptyState v-else title="目录为空" description="文件写入后会显示在此处" />
        <div v-if="filePath" class="space-y-2 rounded-md border p-3">
          <span class="sb-mono text-xs">{{ filePath }}</span>
          <Textarea v-model="fileText" rows="8" aria-label="文件内容" :readonly="sandbox.source === 'agent' || fileBinary" />
          <div class="flex gap-2"><Button v-if="sandbox.source !== 'agent' && !fileBinary" size="sm" :disabled="saving" @click="saveFile">保存</Button><Button size="sm" variant="outline" @click="downloadFile">下载</Button></div>
          <p v-if="fileBinary" class="text-xs text-muted-foreground">二进制文件无法编辑；可下载原始文件。</p>
        </div>
      </template>
      <div v-else class="space-y-2 text-sm">
        <p>状态：{{ sandbox.status }} · 来源：{{ sandbox.source }}</p>
        <p class="sb-mono break-all">ID：{{ sandbox.id }}</p>
        <p class="sb-mono break-all">Cloud：{{ sandbox.cloudName }}</p>
        <p>镜像：{{ sandbox.image }} · {{ sandbox.cpus }}C / {{ sandbox.memoryMiB }}MiB · 网络：{{ sandbox.network }}</p>
        <p>创建于：{{ sandbox.createdAt }} · 最后活动：{{ sandbox.lastActiveAt || '未启动' }}</p>
        <p v-if="sandbox.lastError" class="text-destructive">{{ sandbox.lastError }}</p>
        <Button size="sm" variant="outline" @click="copyExample">复制 curl 示例</Button>
      </div>
    </div>
  </SbModal>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import type { SandboxItem, SandboxExecResult, SandboxFileEntry } from '../../services/types'
import { api } from '../../services/api'
import SbModal from '../modal/SbModal.vue'
import ConfirmAction from '../ConfirmAction.vue'
import SbEmptyState from '../SbEmptyState.vue'

const props = defineProps<{ open: boolean; projectId: string; sandbox: SandboxItem | null; maxFileBytes: number }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; changed: [] }>()
const tab = ref('终端')
const command = ref('')
const running = ref(false)
const saving = ref(false)
const history = ref<(SandboxExecResult & { command: string })[]>([])
const directory = ref('/workspace')
const entries = ref<SandboxFileEntry[]>([])
const filePath = ref('')
const fileText = ref('')
const fileBinary = ref(false)
watch(() => props.sandbox?.id, () => { tab.value = '终端'; history.value = []; directory.value = '/workspace'; entries.value = []; filePath.value = '' })
watch(tab, (value) => { if (value === '文件' && props.open) loadFiles() })
async function execute() {
  if (!props.sandbox || props.sandbox.source === 'agent' || !command.value.trim() || running.value) return
  const text = command.value.trim()
  running.value = true
  try {
    const result = await api.sandboxes.exec(props.projectId, props.sandbox.id, { command: text })
    history.value.unshift({ command: text, ...result })
    command.value = ''
    emit('changed')
  } catch (e) { toast.error(e instanceof Error ? e.message : '执行失败') }
  finally { running.value = false }
}
async function loadFiles() {
  if (!props.sandbox) return
  try { entries.value = await api.sandboxes.files.list(props.projectId, props.sandbox.id, directory.value) }
  catch (e) { toast.error(e instanceof Error ? e.message : '加载文件失败') }
}
async function openEntry(entry: SandboxFileEntry) {
  if (!props.sandbox) return
  if (entry.kind === 'directory') { directory.value = entry.path; filePath.value = ''; await loadFiles(); return }
  filePath.value = entry.path
  try {
    const data = await api.sandboxes.files.read(props.projectId, props.sandbox.id, entry.path)
    fileBinary.value = data.encoding === 'base64'
    fileText.value = fileBinary.value ? '' : data.content
    if (data.truncated) toast.warning('文件内容已截断')
  } catch (e) { toast.error(e instanceof Error ? e.message : '读取失败') }
}
function createFile() { filePath.value = directory.value + '/new.txt'; fileText.value = ''; fileBinary.value = false }
async function saveFile() {
  if (!props.sandbox || saving.value) return
  saving.value = true
  try { await api.sandboxes.files.write(props.projectId, props.sandbox.id, filePath.value, fileText.value); await loadFiles(); toast.success('文件已保存'); emit('changed') }
  catch (e) { toast.error(e instanceof Error ? e.message : '保存失败') }
  finally { saving.value = false }
}
async function removeFile(path: string) {
  if (!props.sandbox) return
  try { await api.sandboxes.files.remove(props.projectId, props.sandbox.id, path); if (filePath.value === path) filePath.value = ''; await loadFiles(); toast.success('文件已删除') }
  catch (e) { toast.error(e instanceof Error ? e.message : '删除失败') }
}
async function uploadFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!props.sandbox || !file) return
  if (file.size > props.maxFileBytes) { toast.error('文件超出大小限制'); return }
  try { await api.sandboxes.files.upload(props.projectId, props.sandbox.id, directory.value + '/' + file.name, new Uint8Array(await file.arrayBuffer())); await loadFiles(); toast.success('上传成功') }
  catch (e) { toast.error(e instanceof Error ? e.message : '上传失败') }
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
  } catch (e) { toast.error(e instanceof Error ? e.message : '下载失败') }
}
async function copyExample() {
  if (!props.sandbox) return
  const example = `curl -X POST '$BASE_URL/v1/projects/${encodeURIComponent(props.projectId)}/sandboxes/${encodeURIComponent(props.sandbox.id)}/exec' -H 'Authorization: Bearer $API_KEY' -H 'Content-Type: application/json' -d '{"command":"echo hello"}'`
  try { await navigator.clipboard.writeText(example); toast.success('curl 示例已复制') }
  catch { toast.error('无法复制到剪贴板') }
}
</script>
