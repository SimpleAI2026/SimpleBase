<template>
  <ProjectScope>
    <PageContainer>
      <Card>
        <CardHeader class="border-b">
          <CardTitle>{{ t('nav.keyValue') }}</CardTitle>
          <CardDescription>
            {{ t('pages.keyValue.subtitle') }}
          </CardDescription>
          <CardAction v-if="!isAdmin">
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="kvLoading" @click="panel?.reload()">
                <Spinner v-if="kvLoading" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                {{ t('common.refresh') }}
              </Button>
              <Button size="sm" @click="panel?.openCreate()">
                <PlusIcon data-icon="inline-start" />
                {{ t('pages.keyValue.createKey') }}
              </Button>
            </div>
          </CardAction>
        </CardHeader>
        <CardContent class="p-3 !pt-2">
          <!-- 系统项目没有项目级 KV：不发请求，直接给空态（planv6.0 §2.2）。 -->
          <SbEmptyState
            v-if="isAdmin"
            :icon="BracesIcon"
            :title="t('pages.keyValue.systemTitle')"
            :description="t('pages.keyValue.systemDesc')"
          />
          <KvPanel
            v-else
            ref="panel"
            :project-id="projectStore.id"
            :readonly="false"
            @loading="kvLoading = $event"
          />
        </CardContent>
      </Card>
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { BracesIcon, PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import ProjectScope from '../components/ProjectScope.vue'
import PageContainer from '../components/PageContainer.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import KvPanel from '../components/databases/kv/KvPanel.vue'
import { useProjectStore } from '../stores/project'

/**
 * Key-Value 项目页：面板跟随左侧导航的项目上下文（key-value-ducklake-plan §4）。
 *
 * admin（系统）项目不展示面板：它没有项目级 KV，`POST /kv` 会返回
 * 404 `kv_store_not_found`。此前会渲染成红色「加载失败」（planv6.0 §2.2）。
 */
const { t } = useI18n()
const projectStore = useProjectStore()
const { isAdmin } = storeToRefs(projectStore)
const kvLoading = ref(false)
const panel = ref<{ reload: () => void; openCreate: () => void } | null>(null)
</script>
