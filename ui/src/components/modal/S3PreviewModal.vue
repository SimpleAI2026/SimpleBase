<template>
  <SbModal
    :open="open"
    :title="info.basename"
    :description="description"
    :max-width="880"
    @update:open="emit('update:open', $event)"
  >
    <div class="flex min-h-[240px] flex-col gap-3">
      <SbAsyncRegion
        block="spinner"
        :pending="pending"
        :show-skeleton="showSkeleton"
        :show-empty="false"
        :show-error="showError"
        :refreshing="refreshing"
        :error="error"
        @retry="load"
      >
        <p v-if="tooLarge" class="rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground">
          {{ t('components.s3Preview.tooLarge', { max: maxPreviewLabel }) }}
        </p>

        <div v-else-if="info.kind === 'image'" class="flex justify-center">
          <img
            :src="url"
            :alt="t('components.s3Preview.imageAlt', { name: info.basename })"
            class="max-h-[58vh] max-w-full rounded-lg border border-border object-contain"
          />
        </div>

        <template v-else-if="isTextLike(info.kind)">
          <div v-if="info.kind === 'markdown'" class="flex justify-end">
            <ToggleGroup
              type="single"
              size="sm"
              :model-value="mdView"
              variant="outline"
              @update:model-value="(v: string | undefined) => v && (mdView = v as MdView)"
            >
              <ToggleGroupItem value="rendered" class="h-7 px-2.5 text-xs">
                {{ t('components.s3Preview.viewRendered') }}
              </ToggleGroupItem>
              <ToggleGroupItem value="source" class="h-7 px-2.5 text-xs">
                {{ t('components.s3Preview.viewSource') }}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>
          <div
            v-if="info.kind === 'markdown' && mdView === 'rendered'"
            class="max-h-[56vh] overflow-auto rounded-lg border border-border px-4 py-3 text-sm leading-7 break-words [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-primary [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:sb-mono [&_code]:text-xs [&_h1]:my-3 [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:my-3 [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:my-2 [&_h3]:font-semibold [&_img]:max-w-full [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-2 [&_pre]:overflow-auto [&_pre]:rounded-md [&_pre]:bg-muted/50 [&_pre]:p-3 [&_table]:w-full [&_td]:border [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:px-2 [&_th]:py-1 [&_ul]:list-disc [&_ul]:pl-5"
            data-testid="s3-preview-markdown"
            v-html="renderedHtml"
          />
          <pre
            v-else
            class="sb-mono max-h-[56vh] overflow-auto rounded-lg border border-border bg-muted/40 p-3 text-xs whitespace-pre-wrap"
          >{{ content }}</pre>
        </template>

        <SbEmptyState
          v-else
          :title="t('components.s3Preview.unsupportedTitle')"
          :description="t('components.s3Preview.unsupportedDesc')"
        />
      </SbAsyncRegion>
    </div>

    <template #footer>
      <div class="flex w-full flex-wrap items-center justify-end gap-2">
        <Button
          v-if="isTextLike(info.kind)"
          variant="outline"
          :disabled="!content || pending"
          @click="copyContent"
        >
          <CopyIcon data-icon="inline-start" />
          {{ t('components.s3Preview.copyContent') }}
        </Button>
        <Button variant="outline" @click="emit('update:open', false)">
          {{ t('common.close') }}
        </Button>
        <Button @click="download">
          <DownloadIcon data-icon="inline-start" />
          {{ t('components.s3Preview.download') }}
        </Button>
      </div>
    </template>
  </SbModal>
</template>

<script setup lang="ts">
/**
 * 对象存储文件预览弹窗：图片（jpg/png/webp/svg…）与文本（md/txt/log…）直接展示。
 *
 * 类型只能按扩展名判断——`S3Object` 不带 content-type。
 * URL 每次打开都重新 presign（S3 预签名 15 分钟过期，本地 FileStore 返回 data: URL）。
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { CopyIcon, DownloadIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import SbModal from './SbModal.vue'
import SbAsyncRegion from '../SbAsyncRegion.vue'
import SbEmptyState from '../SbEmptyState.vue'
import { useLoadState } from '../../composables/useLoadState'
import { renderMarkdown } from '../../docs/render'
import { api } from '../../services/api'
import type { S3Object } from '../../services/api'
import { errorMessage, formatBytes, formatTime } from '../../utils/format'
import { MAX_TEXT_PREVIEW_BYTES, getPreviewInfo, isTextLike } from '../../utils/filePreview'

type MdView = 'rendered' | 'source'

const props = defineProps<{
  open: boolean
  projectId: string
  record?: S3Object | null
}>()

const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t } = useI18n()

const info = computed(() => getPreviewInfo(props.record?.key || ''))
const url = ref('')
const content = ref('')
const mdView = ref<MdView>('rendered')
const tooLarge = computed(
  () => isTextLike(info.value.kind) && (props.record?.size ?? 0) > MAX_TEXT_PREVIEW_BYTES
)
const maxPreviewLabel = computed(() => formatBytes(MAX_TEXT_PREVIEW_BYTES))
const renderedHtml = computed(() =>
  content.value ? renderMarkdown(content.value, 'storage').html : ''
)
const description = computed(() => {
  const record = props.record
  if (!record) return ''
  return t('components.s3Preview.description', {
    key: record.key,
    size: formatBytes(record.size),
    time: formatTime(record.lastModified)
  })
})

const { pending, showSkeleton, showError, refreshing, error, run, reset } = useLoadState({
  fallback: 'components.s3Preview.previewFailed'
})

/** 切换文件 / 关闭弹窗时中止在途请求，避免旧结果覆盖新预览。 */
let controller: AbortController | null = null
let objectUrl = ''

function abort() {
  controller?.abort()
  controller = null
  if (objectUrl) {
    URL.revokeObjectURL(objectUrl)
    objectUrl = ''
  }
}

function load() {
  abort()
  url.value = ''
  content.value = ''
  mdView.value = 'rendered'
  const record = props.record
  if (!record) return Promise.resolve()
  return run(async () => {
    const { url: signed } = await api.s3.presign(props.projectId, record.key)
    url.value = signed
    if (!isTextLike(info.value.kind) || tooLarge.value) return true
    const ctl = new AbortController()
    controller = ctl
    const res = await fetch(signed, { signal: ctl.signal })
    if (!res.ok) throw new Error(t('components.s3Preview.previewFailed'))
    content.value = await res.text()
    return true
  }, { replace: true })
}

async function download() {
  const record = props.record
  if (!record) return
  const name = info.value.basename
  try {
    const { url: signed } = url.value ? { url: url.value } : await api.s3.presign(props.projectId, record.key)
    const res = await fetch(signed)
    if (!res.ok) throw new Error('bad response')
    const blob = await res.blob()
    const href = URL.createObjectURL(blob)
    objectUrl = href
    const a = document.createElement('a')
    a.href = href
    a.download = name
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(href)
    objectUrl = ''
  } catch {
    // 跨域未配置 CORS 时 fetch 会失败，退化回新窗口打开预签名链接
    const signed = url.value || (await api.s3.presign(props.projectId, record.key).then((r) => r.url).catch(() => ''))
    if (signed) {
      window.open(signed, '_blank')
      toast.error(t('components.s3Preview.downloadFailed'))
    } else {
      toast.error(t('pages.s3.linkFailed'))
    }
  }
}

async function copyContent() {
  try {
    await navigator.clipboard.writeText(content.value)
    toast.success(t('components.s3Preview.copied'))
  } catch (e) {
    toast.error(errorMessage(e, t('components.s3Preview.copyFailed')))
  }
}

watch(
  () => [props.open, props.record?.key] as const,
  ([open]) => {
    if (open) void load()
    else {
      abort()
      reset()
    }
  },
  { immediate: true }
)

onBeforeUnmount(abort)
</script>
