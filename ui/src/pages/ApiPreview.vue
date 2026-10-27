<template>
  <PageContainer :subtitle="t('pages.apiPreview.subtitle')">
    <SbAsyncRegion :pending="loading" :show-error="Boolean(loadError)" :error="loadError" @retry="loadDocument">
      <div class="grid min-h-[32rem] gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]">
        <aside class="min-w-0 rounded-xl border bg-card p-3">
          <Input v-model="search" class="mb-3" :placeholder="t('pages.apiPreview.search')" />
          <div class="flex max-h-[42rem] flex-col gap-1 overflow-y-auto">
            <button
              v-for="endpoint in filteredEndpoints"
              :key="endpoint.key"
              type="button"
              class="flex min-w-0 items-center gap-2 rounded-md px-2 py-2 text-left text-xs hover:bg-accent"
              :class="selectedKey === endpoint.key ? 'bg-accent' : ''"
              @click="selectEndpoint(endpoint.key)"
            >
              <span class="w-14 shrink-0 font-mono font-semibold text-primary">{{ endpoint.method }}</span>
              <span class="truncate font-mono text-muted-foreground">{{ endpoint.path }}</span>
            </button>
          </div>
        </aside>

        <main v-if="selected" class="flex min-w-0 flex-col gap-5">
          <div class="rounded-xl border bg-card p-4">
            <div class="mb-4 flex flex-wrap items-center gap-3">
              <span class="rounded bg-primary/10 px-2 py-1 font-mono text-xs font-semibold text-primary">{{ selected.method }}</span>
              <code class="min-w-0 flex-1 break-all text-sm">{{ selected.path }}</code>
            </div>
            <Field v-for="parameter in selected.operation.parameters || []" :key="parameter.name" class="mb-3">
              <FieldLabel>{{ parameter.name }}</FieldLabel>
              <Input v-model="parameters[parameter.name]" :placeholder="parameter.name" />
            </Field>
            <Field class="mb-3">
              <FieldLabel>{{ t('pages.apiPreview.query') }}</FieldLabel>
              <Input v-model="query" placeholder="limit=20&amp;offset=0" />
            </Field>
            <Field v-if="selected.operation.requestBody">
              <FieldLabel>{{ t('pages.apiPreview.body') }}</FieldLabel>
              <textarea v-model="body" class="min-h-28 w-full rounded-lg border bg-background p-3 font-mono text-sm" spellcheck="false" />
            </Field>
            <div class="mt-4 flex items-center gap-3">
              <Button :disabled="!selected.operation['x-tryable'] || running" @click="requestRun">
                {{ running ? t('common.loading') : t('pages.apiPreview.run') }}
              </Button>
              <span v-if="!selected.operation['x-tryable']" class="text-xs text-muted-foreground">{{ t('pages.apiPreview.notTryable') }}</span>
            </div>
            <p v-if="validationError" class="mt-3 text-sm text-destructive">{{ validationError }}</p>
          </div>

          <section v-if="result" class="rounded-xl border bg-card p-4">
            <p class="mb-2 text-sm font-medium">{{ t('pages.apiPreview.response') }} · {{ result.status }}</p>
            <SbCodeBlock :value="result.body" max-height="24rem" />
          </section>

          <section class="grid gap-4 xl:grid-cols-3">
            <div v-for="snippet in snippets" :key="snippet.label" class="min-w-0 rounded-xl border bg-card p-4">
              <p class="mb-2 text-sm font-medium">{{ snippet.label }}</p>
              <SbCodeBlock max-height="20rem">{{ snippet.code }}</SbCodeBlock>
            </div>
          </section>
        </main>
      </div>
    </SbAsyncRegion>
    <ConfirmAction
      controlled
      :open="confirmOpen"
      :title="t('pages.apiPreview.confirmTitle')"
      :description="t('pages.apiPreview.confirmDescription', { method: selected?.method || '', path: resolvedPath })"
      @update:open="confirmOpen = $event"
      @confirm="runRequest"
    />
  </PageContainer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/services/api'
import type { OpenAPIDocument, OpenAPIOperation, APIPreviewResult } from '@/services/types'
import PageContainer from '@/components/PageContainer.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbCodeBlock from '@/components/SbCodeBlock.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'

interface PreviewEndpoint {
  key: string
  method: string
  path: string
  operation: OpenAPIOperation
}

const { t } = useI18n()
const loading = ref(false)
const loadError = ref('')
const document = ref<OpenAPIDocument | null>(null)
const selectedKey = ref('')
const search = ref('')
const query = ref('')
const body = ref('{}')
const parameters = ref<Record<string, string>>({})
const running = ref(false)
const result = ref<APIPreviewResult | null>(null)
const validationError = ref('')
const confirmOpen = ref(false)

const endpoints = computed<PreviewEndpoint[]>(() => {
  const output: PreviewEndpoint[] = []
  for (const [path, operations] of Object.entries(document.value?.paths || {})) {
    for (const [method, operation] of Object.entries(operations)) {
      output.push({ key: `${method.toUpperCase()} ${path}`, method: method.toUpperCase(), path, operation })
    }
  }
  return output.sort((a, b) => a.key.localeCompare(b.key))
})
const filteredEndpoints = computed(() => endpoints.value.filter((endpoint) => endpoint.key.toLowerCase().includes(search.value.toLowerCase())))
const selected = computed(() => endpoints.value.find((endpoint) => endpoint.key === selectedKey.value))
const resolvedPath = computed(() => {
  let path = selected.value?.path || ''
  for (const [name, value] of Object.entries(parameters.value)) {
    path = path.replace(`{${name}}`, encodeURIComponent(value))
  }
  return path + (query.value.trim() ? `?${query.value.trim()}` : '')
})
const snippets = computed(() => {
  if (!selected.value) return []
  const path = resolvedPath.value
  const method = selected.value.method
  const payload = selected.value.operation.requestBody ? body.value : ''
  const curlBody = payload ? ` -d '${payload.replaceAll("'", "'\\''")}'` : ''
  const jsBody = payload ? `,\n  body: ${JSON.stringify(payload)}` : ''
  const goBody = payload ? `strings.NewReader(${JSON.stringify(payload)})` : 'nil'
  return [
    { label: 'cURL', code: `curl -X ${method} "$SIMPLEBASE_URL${path}" -H "Authorization: Bearer $SIMPLEBASE_TOKEN" -H "Content-Type: application/json"${curlBody}` },
    { label: 'JavaScript', code: `await fetch(\`${'${process.env.SIMPLEBASE_URL}'}${path}\`, {\n  method: '${method}',\n  headers: { Authorization: \`Bearer ${'${process.env.SIMPLEBASE_TOKEN}'}\`, 'Content-Type': 'application/json' }${jsBody}\n})` },
    { label: 'Go', code: `req, _ := http.NewRequest("${method}", os.Getenv("SIMPLEBASE_URL")+"${path}", ${goBody})\nreq.Header.Set("Authorization", "Bearer "+os.Getenv("SIMPLEBASE_TOKEN"))\nreq.Header.Set("Content-Type", "application/json")\nresp, err := http.DefaultClient.Do(req)` }
  ]
})

watch(selectedKey, () => {
  parameters.value = Object.fromEntries((selected.value?.operation.parameters || []).map((parameter) => [parameter.name, '']))
  query.value = ''
  body.value = '{}'
  result.value = null
  validationError.value = ''
})

function selectEndpoint(key: string) {
  selectedKey.value = key
}

async function loadDocument() {
  loading.value = true
  loadError.value = ''
  try {
    document.value = await api.preview.document()
    if (!selectedKey.value && endpoints.value.length) selectedKey.value = endpoints.value[0].key
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : t('common.loadFailed')
  } finally {
    loading.value = false
  }
}

function requestRun() {
  validationError.value = ''
  if (!selected.value?.operation['x-tryable']) return
  for (const parameter of selected.value.operation.parameters || []) {
    if (parameter.required && !parameters.value[parameter.name]?.trim()) {
      validationError.value = t('pages.apiPreview.parameterRequired', { name: parameter.name })
      return
    }
  }
  if (selected.value.operation.requestBody) {
    try {
      JSON.parse(body.value)
    } catch {
      validationError.value = t('pages.apiPreview.invalidJSON')
      return
    }
  }
  if (selected.value.method === 'GET') {
    void runRequest()
  } else {
    confirmOpen.value = true
  }
}

async function runRequest() {
  if (!selected.value) return
  running.value = true
  validationError.value = ''
  try {
    const payload = selected.value.operation.requestBody ? JSON.parse(body.value) as unknown : undefined
    result.value = await api.preview.request(selected.value.method, resolvedPath.value, payload)
  } catch (error) {
    validationError.value = error instanceof Error ? error.message : t('common.loadFailed')
  } finally {
    running.value = false
  }
}

onMounted(loadDocument)
</script>
