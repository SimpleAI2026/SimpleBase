<template>
  <div>
    <!-- key 绑定项目 ID：切换项目时强制整页重挂载，避免旧项目数据残留 -->
    <div v-if="store.id" :key="store.id">
      <slot />
    </div>
    <PageContainer v-else subtitle="项目切换仅在右上角">
      <Card>
        <CardContent>
          <SbEmptyState
            description="请先在右上角选择或创建一个项目"
            action-text="创建项目"
            @action="store.openCreateModal()"
          />
        </CardContent>
      </Card>
    </PageContainer>
  </div>
</template>

<script setup lang="ts">
import { useProjectStore } from '../stores/project'
import { Card, CardContent } from '@/components/ui/card'
import PageContainer from './PageContainer.vue'
import SbEmptyState from './SbEmptyState.vue'

const store = useProjectStore()
</script>
