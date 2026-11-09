<template>
  <div class="flex flex-col gap-3">
    <!-- 工具条：搜索 / 类型过滤。刷新与新建在页面标题右侧 -->
    <div class="flex flex-wrap items-center gap-2">
      <Input
        :model-value="pattern"
        :placeholder="t('components.kvPanel.searchPlaceholder')"
        class="min-w-0 flex-1 sm:w-64 sm:flex-none"
        clearable
        @update:model-value="emit('update:pattern', $event)"
        @keydown.enter="emit('reload')"
      />
      <Select :model-value="typeFilter" @update:model-value="(v: string) => emit('update:typeFilter', v)">
        <SelectTrigger class="w-28 shrink-0 sm:w-32"><SelectValue :placeholder="t('components.kvPanel.typePlaceholder')" /></SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ t('components.kvPanel.typeAll') }}</SelectItem>
          <SelectItem value="string">string</SelectItem>
          <SelectItem value="hash">hash</SelectItem>
          <SelectItem value="list">list</SelectItem>
          <SelectItem value="set">set</SelectItem>
          <SelectItem value="zset">zset</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <Alert v-if="readonly">
      <AlertTitle>{{ t('components.kvPanel.readonlyTitle') }}</AlertTitle>
      <AlertDescription>{{ t('components.kvPanel.readonlyDesc') }}</AlertDescription>
    </Alert>

    <!-- 列表 -->
    <div class="overflow-hidden rounded-md border border-border/70">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[38%]">Key</TableHead>
            <TableHead class="w-20">{{ t('components.kvPanel.colType') }}</TableHead>
            <TableHead class="w-24">{{ t('components.kvPanel.colLength') }}</TableHead>
            <TableHead class="w-32">TTL</TableHead>
            <TableHead>{{ t('components.kvPanel.colActions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <SbAsyncRegion
          as="tbody"
          :columns="5"
          :pending="pending"
          :show-skeleton="showSkeleton"
          :show-empty="showEmpty"
          :show-error="showError"
          :refreshing="refreshing"
          :error="error"
          :empty-title="t('components.kvPanel.emptyTitle')"
          :empty-description="t('components.kvPanel.emptyDesc')"
          :empty-action-text="readonly ? undefined : t('components.kvPanel.createKey')"
          @retry="emit('retry')"
          @empty-action="emit('create')"
        >
          <TableRow v-for="m in rows" :key="m.key" class="cursor-pointer" @click="emit('detail', m)">
            <TableCell class="sb-mono max-w-0 truncate">{{ m.key }}</TableCell>
            <TableCell>
              <Badge :variant="typeBadge(m.type)">{{ m.type }}</Badge>
            </TableCell>
            <TableCell class="sb-mono">{{ m.len ?? '—' }}</TableCell>
            <TableCell class="text-muted-foreground">{{ ttlText(m) }}</TableCell>
            <TableCell>
              <div class="inline-flex gap-1" @click.stop>
                <Button size="sm" variant="ghost" :disabled="readonly" @click="emit('ttl', m)">TTL</Button>
                <Button size="sm" variant="ghost" :disabled="readonly" @click="emit('rename', m)">{{ t('components.kvPanel.rename') }}</Button>
                <ConfirmAction
                  :title="t('components.kvPanel.delete') + ' Key'"
                  :description="t('components.kvPanel.deleteConfirm', { key: m.key })"
                  :disabled="readonly"
                  @confirm="emit('remove', m)"
                >
                  <Button size="sm" variant="ghost" class="text-destructive hover:text-destructive">{{ t('components.kvPanel.delete') }}</Button>
                </ConfirmAction>
              </div>
            </TableCell>
          </TableRow>
        </SbAsyncRegion>
      </Table>
    </div>

    <div v-if="hasMore" class="flex justify-center">
      <Button size="sm" variant="outline" :disabled="pending || loadingMore" @click="emit('load-more')">
        <Spinner v-if="loadingMore" class="mr-2 h-3.5 w-3.5" />
        {{ t('components.kvPanel.loadMore') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * KV 键列表（工具条 + 表格 + 加载更多），从 KvPanel 拆出
 * （plan/planv5.0/ui-optimization-v3-plan.md §3.6）。
 *
 * 拆分边界刻意选在「纯展示」：`rows` / `pattern` / `typeFilter` 等状态仍由
 * KvPanel 持有，这里只通过 props 读、通过 emit 写。这样既缩短了宿主模板，
 * 又不动既有测试依赖的 `vm.rows` 等表面。
 */
import { useI18n } from 'vue-i18n'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Table, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import SbAsyncRegion from '../../SbAsyncRegion.vue'
import ConfirmAction from '../../ConfirmAction.vue'
import type { KvKeyMeta } from '../../../services/api'

defineProps<{
  rows: KvKeyMeta[]
  pattern: string
  typeFilter: string
  readonly?: boolean
  hasMore: boolean
  loadingMore: boolean
  pending: boolean
  showSkeleton: boolean
  showEmpty: boolean
  showError: boolean
  refreshing: boolean
  error: string
  /** TTL 展示格式化（宿主持有倒计时计时器，故由此注入）。 */
  ttlText: (m: KvKeyMeta) => string
}>()

const emit = defineEmits<{
  'update:pattern': [value: string]
  'update:typeFilter': [value: string]
  reload: []
  create: []
  retry: []
  'load-more': []
  detail: [m: KvKeyMeta]
  ttl: [m: KvKeyMeta]
  rename: [m: KvKeyMeta]
  remove: [m: KvKeyMeta]
}>()

const { t } = useI18n()

function typeBadge(type: string): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (type) {
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
</script>
