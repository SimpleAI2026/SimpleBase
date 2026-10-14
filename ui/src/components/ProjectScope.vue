<template>
  <div>
    <!-- key 绑定项目 ID：切换项目时强制整页重挂载，避免旧项目数据残留 -->
    <div v-if="store.id" :key="store.id">
      <slot />
    </div>
    <PageContainer v-else :subtitle="t('common.projectSwitchOnlyTopRight')">
      <Card>
        <CardContent>
          <SbEmptyState
            :description="t('common.projectRequired')"
            :action-text="t('common.createProject')"
            @action="store.openCreateModal()"
          />
        </CardContent>
      </Card>
    </PageContainer>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useProjectStore } from '../stores/project'
import { Card, CardContent } from '@/components/ui/card'
import PageContainer from './PageContainer.vue'
import SbEmptyState from './SbEmptyState.vue'

const store = useProjectStore()
const { t } = useI18n()
</script>
