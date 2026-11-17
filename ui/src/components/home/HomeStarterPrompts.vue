<template>
  <!-- 首页「试试这样说」提示词卡片（planv6.6 home-starter-prompts §2.1）。
       点击只 emit pick，由 Home 打开确认弹窗；这里不做任何建项目动作。 -->
  <ul class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" data-starter-grid>
    <li v-for="tpl in templates" :key="tpl.id">
      <button
        type="button"
        class="flex h-full min-h-32 w-full flex-col gap-3 rounded-xl border border-border bg-card p-4 text-left transition-colors hover:border-primary/40 hover:bg-primary/5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:p-5"
        :data-starter="tpl.id"
        @click="emit('pick', tpl.id)"
      >
        <span class="flex items-center gap-2.5">
          <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <component :is="iconOf(tpl.icon)" class="size-4" aria-hidden="true" />
          </span>
          <span class="text-sm font-medium text-foreground">{{ t(tpl.titleKey) }}</span>
        </span>
        <span class="text-xs leading-5 text-text-secondary">{{ t(tpl.descKey) }}</span>
        <span class="mt-auto flex flex-wrap gap-1.5">
          <Badge v-for="o in tpl.outputs" :key="o" variant="secondary" :data-output="o">{{ t(`pages.home.starterOutputs.${o}`) }}</Badge>
        </span>
      </button>
    </li>
  </ul>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChartColumnIcon, MegaphoneIcon, UserRoundIcon } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { STARTER_TEMPLATES, type StarterTemplate } from '@/constants/starter-prompts'

const emit = defineEmits<{ pick: [templateId: string] }>()

const { t } = useI18n()
const templates = STARTER_TEMPLATES

const ICONS: Record<StarterTemplate['icon'], Component> = {
  dashboard: ChartColumnIcon,
  portfolio: UserRoundIcon,
  notice: MegaphoneIcon
}

function iconOf(name: StarterTemplate['icon']): Component {
  return ICONS[name]
}
</script>
