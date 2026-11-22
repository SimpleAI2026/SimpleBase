<template>
  <SbListCard :title="t('components.mcpServers.builtin.title')" :description="t('components.mcpServers.builtin.desc')">
    <div class="p-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="sb-col-name">{{ t('components.mcpServers.colName') }}</TableHead>
            <TableHead class="min-w-48">{{ t('components.mcpServers.builtin.endpoint') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('components.mcpServers.colTools') }}</TableHead>
            <TableHead class="sb-col-sm">{{ t('common.enabledSw') }}</TableHead>
            <TableHead class="sb-col-act min-w-48">{{ t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-for="item in servers" :key="item.name">
            <TableRow>
              <TableCell>
                <div class="flex items-center gap-2">
                  <span class="sb-mono text-sm font-medium">{{ item.name }}</span>
                  <Badge variant="secondary">{{ t('components.mcpServers.builtin.badge') }}</Badge>
                </div>
              </TableCell>
              <TableCell>
                <div class="flex items-center gap-1">
                  <span class="sb-mono truncate text-xs" :title="item.url">{{ item.url }}</span>
                  <Button
                    size="sm"
                    variant="ghost"
                    :aria-label="t('components.mcpServers.builtin.copy')"
                    @click="copyUrl(item)"
                  >
                    <CopyIcon data-icon="inline-start" />
                  </Button>
                </div>
              </TableCell>
              <TableCell>
                <Button size="sm" variant="ghost" @click="toggleTools(item.name)">
                  {{ item.tools.length }}
                </Button>
              </TableCell>
              <TableCell>
                <Switch
                  :model-value="item.enabled"
                  :disabled="!auth.canWrite || toggling.has(item.name)"
                  @update:model-value="toggle(item, $event)"
                />
              </TableCell>
              <TableCell>
                <div class="flex flex-wrap justify-center gap-1">
                  <Button
                    v-if="auth.canWrite"
                    size="sm"
                    variant="ghost"
                    :disabled="testing.has(item.name)"
                    @click="test(item)"
                  >
                    <Spinner v-if="testing.has(item.name)" data-icon="inline-start" />
                    {{ t('components.mcpServers.test') }}
                  </Button>
                  <Button size="sm" variant="ghost" @click="configuring = item">
                    {{ t('components.mcpServers.builtin.configure') }}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
            <TableRow v-if="expanded.has(item.name)">
              <TableCell colspan="5">
                <div class="flex flex-wrap gap-2">
                  <Badge v-for="tool in item.tools" :key="tool.name" variant="outline" class="sb-mono">
                    {{ tool.name }}
                    <span v-if="tool.destructive" class="text-destructive">
                      {{ t('components.mcpServers.builtin.destructive') }}
                    </span>
                    <span v-else-if="tool.read_only" class="text-muted-foreground">
                      {{ t('components.mcpServers.builtin.readOnly') }}
                    </span>
                  </Badge>
                </div>
              </TableCell>
            </TableRow>
          </template>
        </TableBody>
      </Table>
    </div>

    <SbModal
      :open="configuring !== null"
      :title="t('components.mcpServers.builtin.configure')"
      :description="t('components.mcpServers.builtin.configureDesc')"
      :max-width="640"
      hide-footer
      @update:open="(v: boolean) => { if (!v) configuring = null }"
    >
      <div v-if="configuring" class="flex flex-col gap-4">
        <div v-for="snippet in snippets(configuring)" :key="snippet.label" class="flex flex-col gap-1.5">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium">{{ snippet.label }}</span>
            <Button size="sm" variant="ghost" @click="copySnippet(snippet.body)">
              <CopyIcon data-icon="inline-start" />
              {{ t('common.copy') }}
            </Button>
          </div>
          <SbCodeBlock max-height="14rem">{{ snippet.body }}</SbCodeBlock>
        </div>
        <p class="text-xs text-muted-foreground">
          {{ t('components.mcpServers.builtin.keyHint') }}
          <button type="button" class="underline" @click="openConnection">
            {{ t('components.mcpServers.builtin.keyLink') }}
          </button>
        </p>
      </div>
    </SbModal>
  </SbListCard>
</template>

<script setup lang="ts">
/**
 * 内置 MCP Server 区块（planv6.7 §3.5）：项目 API 封装出的 Server，只读展示，
 * 只允许开关、测试与查看接入配置，不提供编辑和删除。
 * 接入片段只放占位符，不嵌入真实令牌。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CopyIcon } from '@lucide/vue'
import { api } from '@/services/api'
import type { McpBuiltinServer } from '@/services/types'
import { useAuthStore } from '@/stores/auth'
import { copyText } from '@/utils/clipboard'
import { errorMessage } from '@/utils/format'
import SbCodeBlock from '@/components/SbCodeBlock.vue'
import SbListCard from '@/components/SbListCard.vue'
import SbModal from '@/components/modal/SbModal.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const props = defineProps<{
  projectId: string
  servers: McpBuiltinServer[]
}>()
const emit = defineEmits<{ changed: [] }>()

const { t } = useI18n()
const auth = useAuthStore()

const toggling = ref(new Set<string>())
const testing = ref(new Set<string>())
const expanded = ref(new Set<string>())
const configuring = ref<McpBuiltinServer | null>(null)

function toggleTools(name: string) {
  if (expanded.value.has(name)) expanded.value.delete(name)
  else expanded.value.add(name)
}

function copyUrl(item: McpBuiltinServer) {
  return copyText(item.url, t('components.mcpServers.builtin.copied'), t('common.copyFailed'))
}

function copySnippet(body: string) {
  return copyText(body, t('components.mcpServers.builtin.copied'), t('common.copyFailed'))
}

function snippets(item: McpBuiltinServer): { label: string; body: string }[] {
  const headers = { Authorization: 'Bearer <SIMPLEBASE_API_KEY>' }
  const claude = { mcpServers: { [item.name]: { url: item.url, headers } } }
  const cursor = { mcpServers: { [item.name]: { url: item.url, headers } } }
  const generic = { mcpServers: { [item.name]: { type: 'http', url: item.url, headers } } }
  return [
    { label: 'Claude Desktop', body: JSON.stringify(claude, null, 2) },
    { label: 'Cursor', body: JSON.stringify(cursor, null, 2) },
    { label: 'mcp.json', body: JSON.stringify(generic, null, 2) }
  ]
}

function openConnection() {
  configuring.value = null
  auth.openSettings({ tab: 'connection' })
}

async function toggle(item: McpBuiltinServer, enabled: boolean) {
  if (!auth.canWrite || toggling.value.has(item.name)) return
  toggling.value.add(item.name)
  try {
    await api.mcpServers.setBuiltinEnabled(props.projectId, item.name, enabled)
    toast.success(t('components.mcpServers.saved'))
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServers.saveFailed')))
  } finally {
    toggling.value.delete(item.name)
  }
}

async function test(item: McpBuiltinServer) {
  if (testing.value.has(item.name)) return
  testing.value.add(item.name)
  try {
    const res = await api.mcpServers.testBuiltin(props.projectId, item.name)
    if (res.ok) toast.success(t('components.mcpServers.builtin.testOk', { n: res.tools.length }))
    else toast.error(t('components.mcpServers.testFailed'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.mcpServers.testFailed')))
  } finally {
    testing.value.delete(item.name)
  }
}
</script>
