<template>
  <div class="project-scope">
    <slot v-if="store.id" />
    <PageContainer v-else title="未选择项目" subtitle="项目切换仅在右上角">
      <a-card class="sb-card">
        <SbEmptyState
          description="请先在右上角选择或创建一个项目"
          action-text="创建项目"
          @action="store.openCreateModal()"
        />
      </a-card>
    </PageContainer>
  </div>
</template>

<script setup lang="ts">
import { useProjectStore } from '../stores/project'
import PageContainer from './PageContainer.vue'
import SbEmptyState from './SbEmptyState.vue'

const store = useProjectStore()
</script>
