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
            <p v-if="!filteredEndpoints.length" class="px-2 py-6 text-center text-xs text-muted-foreground">{{ t('common.noMatchFilter') }}</p>
          </div>
        </aside>

        <!-- SidebarInset 已是 <main>：这里用 section，避免地标重复（B15） -->
        <section v-if="selected" class="flex min-w-0 flex-col gap-5">
          <div class="rounded-xl border bg-card p-4">
            <div class="mb-4 flex flex-wrap items-center gap-3">
              <span class="rounded bg-primary/10 px-2 py-1 font-mono text-xs font-semibold text-primary">{{ selected.method }}</span>
              <code class="min-w-0 flex-1 break-all text-sm">{{ selected.path }}</code>
            </div>
            <Field v-for="parameter in selected.operation.parameters || []" :key="parameter.name" class="mb-3">
              <FieldLabel>{{ parameter.name }}</FieldLabel>
              <template v-if="parameter['x-sb-source']">
                <Combobox
                  :model-value="parameters[parameter.name]"
                  :disabled="!canPickParam(parameter['x-sb-source'])"
                  @update:model-value="onParamCommit(parameter.name, String($event ?? ''))"
                >
                  <ComboboxAnchor>
                    <ComboboxInput
                      :placeholder="parameter.name"
                      @input="onParamInput(parameter.name, $event)"
                      @change="onParamCommit(parameter.name, ($event.target as HTMLInputElement).value)"
                    />
                  </ComboboxAnchor>
                  <ComboboxList>
                    <ComboboxViewport>
                      <ComboboxEmpty>{{ t('common.noMatchFilter') }}</ComboboxEmpty>
                      <ComboboxGroup>
                        <ComboboxItem
                          v-for="opt in paramOptionState[parameter.name]?.options || []"
                          :key="opt.value"
                          :value="opt.value"
                        >
                          <span class="flex-1 truncate">{{ opt.label }}</span>
                          <span v-if="opt.hint" class="ml-2 font-mono text-xs text-muted-foreground">{{ opt.hint }}</span>
                        </ComboboxItem>
                      </ComboboxGroup>
                    </ComboboxViewport>
                  </ComboboxList>
                </Combobox>
                <p v-if="paramOptionState[parameter.name]?.error" class="mt-1 text-xs text-muted-foreground">
                  {{ t('pages.apiPreview.paramOptionsFailed') }}
                </p>
                <p v-else-if="!canPickParam(parameter['x-sb-source'])" class="mt-1 text-xs text-muted-foreground">
                  {{ t('pages.apiPreview.paramDependsOn', { params: (parameter['x-sb-source']?.depends_on || []).join(', ') }) }}
                </p>
              </template>
              <Input v-else v-model="parameters[parameter.name]" :placeholder="parameter.name" />
            </Field>
            <Field class="mb-3">
              <FieldLabel>{{ t('pages.apiPreview.query') }}</FieldLabel>
              <Input v-model="query" placeholder="limit=20&amp;offset=0" />
            </Field>
            <Field v-if="selected.operation.requestBody">
              <FieldLabel>{{ t('pages.apiPreview.body') }}</FieldLabel>
              <Textarea v-model="body" class="min-h-28 font-mono text-sm" spellcheck="false" />
            </Field>
            <div class="mt-4 flex items-center gap-3">
              <Button size="sm" :disabled="!selected.operation['x-tryable'] || running" @click="requestRun">
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
        </section>
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/services/api'
import type { OpenAPIDocument, OpenAPIOperation, OpenAPIParamSource, APIPreviewResult } from '@/services/types'
import { useApiParamOptions, type ParamOption } from '@/composables/useApiParamOptions'
import { useProjectStore } from '@/stores/project'
import PageContainer from '@/components/PageContainer.vue'
import SbAsyncRegion from '@/components/SbAsyncRegion.vue'
import SbCodeBlock from '@/components/SbCodeBlock.vue'
import ConfirmAction from '@/components/ConfirmAction.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Combobox,
  ComboboxAnchor,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxViewport,
} from '@/components/ui/combobox'

interface PreviewEndpoint {
  key: string
  method: string
  path: string
  operation: OpenAPIOperation
}

interface ParamOptionState {
  options: ParamOption[]
  error: boolean
}

const { t } = useI18n()
const projectStore = useProjectStore()
const projectIdRef = computed(() => projectStore.id)
const { options: loadOptions } = useApiParamOptions(projectIdRef)
const paramOptionState = reactive<Record<string, ParamOptionState>>({})
const loading = ref(false)
const loadError = ref('')
const openapiDoc = ref<OpenAPIDocument | null>(null)
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
  for (const [path, operations] of Object.entries(openapiDoc.value?.paths || {})) {
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
    path = path.replaceAll(`{${name}}`, encodeURIComponent(value))
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

const REMEMBER_PREFIX = 'sb.apiPreview.params.'

function rememberKey(): string {
  return REMEMBER_PREFIX + projectIdRef.value
}

/** 同一会话内按 kind 记住上次选择的值。 */
function readRemembered(): Record<string, string> {
  try {
    const raw = sessionStorage.getItem(rememberKey())
    return raw ? (JSON.parse(raw) as Record<string, string>) : {}
  } catch {
    return {}
  }
}

function remember(kind: string, value: string) {
  if (!value) return
  try {
    sessionStorage.setItem(rememberKey(), JSON.stringify({ ...readRemembered(), [kind]: value }))
  } catch {
    // sessionStorage 不可用时只是不记忆，不影响请求
  }
}

/** 依赖参数当前值，键为参数名。 */
function depValues(source: OpenAPIParamSource): Record<string, string> {
  return Object.fromEntries((source.depends_on || []).map((name) => [name, parameters.value[name] || '']))
}

function canPickParam(source?: OpenAPIParamSource): boolean {
  if (!source) return true
  return (source.depends_on || []).every((name) => Boolean(parameters.value[name]?.trim()))
}

/** 拉取某个参数的候选项；失败只标记该参数，下拉退化为自由输入。 */
async function refreshParamOptions(name: string, source: OpenAPIParamSource): Promise<ParamOption[]> {
  if (!canPickParam(source)) {
    paramOptionState[name] = { options: [], error: false }
    return []
  }
  try {
    const list = await loadOptions(source.kind, depValues(source))
    paramOptionState[name] = { options: list, error: false }
    return list
  } catch {
    paramOptionState[name] = { options: [], error: true }
    return []
  }
}

/** 为当前端点的所有带来源参数注入默认值并加载候选项；按依赖顺序串行，保证依赖值先就位。 */
async function injectDefaults() {
  const params = selected.value?.operation.parameters || []
  const remembered = readRemembered()
  const token = selectedKey.value + '|' + projectIdRef.value
  const ordered = [...params].sort(
    (a, b) => (a['x-sb-source']?.depends_on?.length || 0) - (b['x-sb-source']?.depends_on?.length || 0)
  )
  for (const parameter of ordered) {
    const source = parameter['x-sb-source']
    if (!source) continue
    const list = await refreshParamOptions(parameter.name, source)
    if (token !== selectedKey.value + '|' + projectIdRef.value) return
    if (parameters.value[parameter.name]) continue
    if (source.kind === 'project') {
      parameters.value[parameter.name] = projectIdRef.value
      continue
    }
    const last = remembered[source.kind]
    if (last && list.some((opt) => opt.value === last)) {
      parameters.value[parameter.name] = last
    } else if (list.length === 1) {
      parameters.value[parameter.name] = list[0].value
    }
    if (parameters.value[parameter.name]) await refreshDependents(parameter.name)
  }
}

/** 某参数变化后，清空依赖它的参数并重新拉取选项。 */
async function refreshDependents(changed: string) {
  for (const parameter of selected.value?.operation.parameters || []) {
    const source = parameter['x-sb-source']
    if (!source?.depends_on?.includes(changed)) continue
    parameters.value[parameter.name] = ''
    await refreshParamOptions(parameter.name, source)
  }
}

function onParamInput(name: string, event: Event) {
  parameters.value[name] = (event.target as HTMLInputElement).value
}

/** 选中或失焦确认后：记忆取值、清空并刷新依赖项。 */
function onParamCommit(name: string, value: string) {
  const changed = parameters.value[name] !== value
  parameters.value[name] = value
  const source = selected.value?.operation.parameters?.find((p) => p.name === name)?.['x-sb-source']
  if (source) remember(source.kind, value)
  if (changed) void refreshDependents(name)
}

watch(selectedKey, () => {
  parameters.value = Object.fromEntries((selected.value?.operation.parameters || []).map((parameter) => [parameter.name, '']))
  for (const key of Object.keys(paramOptionState)) delete paramOptionState[key]
  query.value = ''
  body.value = '{}'
  result.value = null
  validationError.value = ''
  void injectDefaults()
})

watch(projectIdRef, () => {
  parameters.value = Object.fromEntries(Object.keys(parameters.value).map((name) => [name, '']))
  for (const key of Object.keys(paramOptionState)) delete paramOptionState[key]
  void injectDefaults()
})

function selectEndpoint(key: string) {
  selectedKey.value = key
}

async function loadDocument() {
  loading.value = true
  loadError.value = ''
  try {
    openapiDoc.value = await api.preview.document()
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
