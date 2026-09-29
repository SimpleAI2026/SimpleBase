<template>
  <Tabs v-model="tab" class="gap-3">
    <TabsList>
      <TabsTrigger value="collections">集合文档</TabsTrigger>
    </TabsList>
    <TabsContent value="collections">
      <CollectionPanel
        :project-id="projectId"
        :database="database"
        :reload-token="reloadToken"
        :readonly="readonly"
        @view-data="(c) => emit('view-data', c)"
        @add-document="(c) => emit('add-document', c)"
        @create-collection="emit('create-collection')"
      />
    </TabsContent>
  </Tabs>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { DatabaseItem } from '../../services/api'
import CollectionPanel from './CollectionPanel.vue'

/** 数据库「查看数据」展开行容器：集合文档页签。
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

const tab = ref<'collections'>('collections')
</script>
