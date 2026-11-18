<template>
  <ProjectScope>
    <PageContainer :subtitle="t('pages.keyValue.subtitle')">
      <SbListCard :title="t('nav.keyValue')">
        <template #actions>
          <template v-if="!isAdmin">
            <SbRefreshButton :pending="kvLoading" @click="panel?.reload()" />
            <Button v-if="auth.canWrite" size="sm" @click="panel?.openCreate()">
              <PlusIcon data-icon="inline-start" />
              {{ t('pages.keyValue.createKey') }}
            </Button>
          </template>
        </template>
        <div class="p-3 !pt-2">
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
            :readonly="!auth.canWrite"
            @loading="kvLoading = $event"
          />
        </div>
      </SbListCard>
    </PageContainer>
  </ProjectScope>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { BracesIcon, PlusIcon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import ProjectScope from '../components/ProjectScope.vue'
import PageContainer from '../components/PageContainer.vue'
import SbEmptyState from '../components/SbEmptyState.vue'
import SbRefreshButton from '../components/SbRefreshButton.vue'
import SbListCard from '../components/SbListCard.vue'
import KvPanel from '../components/databases/kv/KvPanel.vue'
import { useProjectStore } from '../stores/project'
import { useAuthStore } from '../stores/auth'

/**
 * Key-Value 项目页：面板跟随左侧导航的项目上下文（key-value-ducklake-plan §4）。
 *
 * admin（系统）项目不展示面板：它没有项目级 KV，`POST /kv` 会返回
 * 404 `kv_store_not_found`。此前会渲染成红色「加载失败」（planv6.0 §2.2）。
 * 副标题走 PageContainer，与其他页面同一结构（planv6.7 ui-simplify-ux-plan R6）；
 * 只读角色隐藏新建并让面板进入只读（B11）。
 */
const { t } = useI18n()
const projectStore = useProjectStore()
const auth = useAuthStore()
const { isAdmin } = storeToRefs(projectStore)
const kvLoading = ref(false)
const panel = ref<{ reload: () => void; openCreate: () => void } | null>(null)
</script>
