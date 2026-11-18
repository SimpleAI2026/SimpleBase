<template>
  <!-- 只剩集合一个分组：不再套 Tabs 外壳（planv6.7 ui-simplify-ux-plan C7），保留分组标题 -->
  <div class="flex flex-col gap-3">
    <p class="m-0 text-sm font-medium text-foreground">{{ t('components.dataTabs.collections') }}</p>
    <CollectionPanel
      :project-id="projectId"
      :database="database"
      :reload-token="reloadToken"
      :readonly="readonly"
      @view-data="(c) => emit('view-data', c)"
      @add-document="(c) => emit('add-document', c)"
      @create-collection="emit('create-collection')"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { DatabaseItem } from '../../services/api'
import CollectionPanel from './CollectionPanel.vue'

const { t } = useI18n()

/** 数据库「查看数据」展开行容器：集合文档。
 *  Key-Value 已改为项目级（左侧导航「Key-Value」页），不再挂在数据库下。 */
defineProps<{
  projectId: string
  database: DatabaseItem
  reloadToken?: number
  readonly?: boolean
}>()

const emit = defineEmits<{
  'view-data': [collection: string]
  'add-document': [collection: string]
  'create-collection': []
}>()
</script>
