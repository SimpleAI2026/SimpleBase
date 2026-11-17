<template>
  <!-- 交付卡片（planv6.6 home-starter-prompts §2.8）。
       只信结构不信 URL：站点地址必须与后端 baseUrl 同源，函数地址由前端按当前 origin 重新拼接。
       字段缺失逐项隐藏。 -->
  <Card size="sm" data-starter-delivery class="border-success/40 bg-success/5">
    <CardHeader>
      <CardTitle class="flex items-center gap-1.5 text-sm">
        <PackageCheckIcon class="size-4 text-success" aria-hidden="true" />
        {{ t('components.starter.delivery.title') }}
      </CardTitle>
    </CardHeader>
    <CardContent class="flex flex-col gap-3 text-xs">
      <section v-if="siteUrl" data-delivery="site" class="flex flex-col gap-1">
        <span class="text-muted-foreground">{{ t('components.starter.delivery.site') }}</span>
        <span class="flex flex-wrap items-center gap-2">
          <code class="sb-mono min-w-0 break-all text-foreground">{{ siteUrl }}</code>
          <Button as="a" size="xs" variant="outline" :href="siteUrl" target="_blank" rel="noopener noreferrer" data-action="open">{{ t('components.starter.delivery.open') }}</Button>
          <Button size="xs" variant="ghost" data-action="copy-site" @click="copy(siteUrl)">{{ t('components.starter.delivery.copy') }}</Button>
        </span>
      </section>
      <section v-else-if="siteSkipped" data-delivery="site-skipped" class="text-muted-foreground">
        {{ t('components.starter.delivery.siteSkipped') }}
      </section>

      <section v-if="functionRows.length" data-delivery="functions" class="flex flex-col gap-1">
        <span class="text-muted-foreground">{{ t('components.starter.delivery.functions') }}</span>
        <ul class="m-0 flex list-none flex-col gap-1 p-0">
          <li v-for="f in functionRows" :key="f.name" class="flex flex-wrap items-center gap-2">
            <code class="sb-mono min-w-0 break-all text-foreground">POST {{ f.url }}</code>
            <Button size="xs" variant="ghost" data-action="copy-curl" @click="copy(f.curl)">{{ t('components.starter.delivery.copyCurl') }}</Button>
          </li>
        </ul>
      </section>

      <section v-if="delivery.tables.length || delivery.collections.length" data-delivery="data" class="flex flex-col gap-1">
        <span class="text-muted-foreground">{{ t('components.starter.delivery.data') }}</span>
        <span class="flex flex-wrap gap-1.5">
          <Badge v-for="x in delivery.tables" :key="'t:' + x" variant="secondary">{{ x }}</Badge>
          <Badge v-for="x in delivery.collections" :key="'c:' + x" variant="outline">{{ x }}</Badge>
        </span>
      </section>

      <section v-if="delivery.cron.length" data-delivery="cron" class="flex flex-col gap-1">
        <span class="text-muted-foreground">{{ t('components.starter.delivery.cron') }}</span>
        <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
          <li v-for="c in delivery.cron" :key="c.name" class="sb-mono text-foreground">{{ c.name }}<span v-if="c.expr" class="text-muted-foreground"> · {{ c.expr }}</span></li>
        </ul>
      </section>

      <section v-if="delivery.next.length" data-delivery="next" class="flex flex-col gap-1">
        <span class="text-muted-foreground">{{ t('components.starter.delivery.next') }}</span>
        <ul class="m-0 list-disc pl-4 text-text-secondary">
          <li v-for="n in delivery.next" :key="n">{{ n }}</li>
        </ul>
      </section>

      <nav class="flex flex-wrap gap-x-3 gap-y-1 border-t border-border pt-2" :aria-label="t('components.starter.delivery.resources')">
        <router-link v-for="l in resourceLinks" :key="l.to" :to="l.to" class="text-foreground underline decoration-border underline-offset-4 hover:decoration-foreground" :data-resource="l.to">{{ t(l.label) }}</router-link>
      </nav>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { PackageCheckIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { functionCallUrl, resolveSiteUrl, type StarterDelivery } from '@/utils/starter-delivery'

const props = defineProps<{
  delivery: StarterDelivery
  projectId: string
  /** 后端 GET /s3/site 的 baseUrl；为空表示站点未启用，助手给的站点地址一律丢弃。 */
  siteBaseUrl?: string
  /** 本次搭建是否跳过了站点阶段（不可发布）。 */
  siteSkipped?: boolean
  /** 页面 origin，默认取当前窗口；测试可注入。 */
  origin?: string
}>()

const { t } = useI18n()

const siteUrl = computed(() => resolveSiteUrl(props.delivery.siteUrl, props.siteBaseUrl))
const origin = computed(() => props.origin ?? (typeof location !== 'undefined' ? location.origin : ''))

const functionRows = computed(() =>
  props.delivery.functions.map((f) => {
    const url = functionCallUrl(origin.value, props.projectId, f)
    // 示例里不放任何密钥：用占位符，由用户自己替换。
    const curl = `curl -X POST '${url}' -H 'Authorization: Bearer <API_KEY>' -H 'Content-Type: application/json' -d '{}'`
    return { name: f.name, url, curl }
  })
)

/** 「查看项目资源」入口（计划 §2.8）。 */
const resourceLinks = [
  { to: '/console/databases', label: 'components.starter.delivery.linkDatabases' },
  { to: '/console/gofunctions', label: 'components.starter.delivery.linkFunctions' },
  { to: '/console/cron-jobs', label: 'components.starter.delivery.linkCron' },
  { to: '/console/s3', label: 'components.starter.delivery.linkS3' }
]

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('components.starter.delivery.copied'))
  } catch {
    toast.error(t('components.starter.delivery.copyFailed'))
  }
}
</script>
