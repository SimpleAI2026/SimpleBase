<template>
  <div>
    <slot v-if="store.id" />
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
