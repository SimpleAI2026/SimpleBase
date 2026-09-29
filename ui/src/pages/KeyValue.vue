<template>
  <ProjectScope>
    <PageContainer>
      <Card>
        <CardHeader class="border-b">
          <CardTitle>Key-Value</CardTitle>
          <CardDescription>
            Redis 语义的 Key-Value 数据服务：string / hash / list / set / zset。
          </CardDescription>
          <CardAction v-if="!isAdmin">
            <div class="flex items-center gap-2">
              <Button variant="outline" size="sm" :disabled="kvLoading" @click="panel?.reload()">
                <Spinner v-if="kvLoading" data-icon="inline-start" />
                <RefreshCwIcon v-else data-icon="inline-start" />
                刷新
              </Button>
              <Button size="sm" @click="panel?.openCreate()">
                <PlusIcon data-icon="inline-start" />
                新建 Key
              </Button>
            </div>
          </CardAction>
          <CardAction v-else>
            <Button variant="outline" size="sm" :disabled="kvLoading" @click="panel?.reload()">
              <Spinner v-if="kvLoading" data-icon="inline-start" />
              <RefreshCwIcon v-else data-icon="inline-start" />
              刷新
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent class="p-3 !pt-2">
          <KvPanel
            ref="panel"
            :project-id="projectStore.id"
            :readonly="isAdmin"
            @loading="kvLoading = $event"
          />
        </CardContent>
      </Card>
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { PlusIcon, RefreshCwIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import ProjectScope from '../components/ProjectScope.vue'
import PageContainer from '../components/PageContainer.vue'
import KvPanel from '../components/databases/kv/KvPanel.vue'
import { useProjectStore } from '../stores/project'

/** Key-Value 项目页：面板跟随左侧导航的项目上下文（key-value-ducklake-plan §4）。 */
const projectStore = useProjectStore()
const { isAdmin } = storeToRefs(projectStore)
const kvLoading = ref(false)
const panel = ref<{ reload: () => void; openCreate: () => void } | null>(null)
</script>
