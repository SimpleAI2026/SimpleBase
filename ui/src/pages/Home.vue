<template>
  <div class="relative min-h-svh overflow-hidden bg-background text-foreground">
    <div aria-hidden="true" class="pointer-events-none absolute inset-x-0 top-0 h-[760px] bg-[radial-gradient(ellipse_at_70%_20%,color-mix(in_srgb,var(--brand)_13%,transparent),transparent_57%)]" />
    <div aria-hidden="true" class="pointer-events-none absolute inset-x-0 top-0 h-[850px] opacity-35 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] bg-[size:52px_52px] [mask-image:linear-gradient(to_bottom,black,transparent)]" />

    <header class="relative z-10 border-b border-border/70 bg-background/75 backdrop-blur-xl">
      <nav aria-label="主导航" class="mx-auto flex h-16 w-full max-w-7xl items-center justify-between gap-3 px-4 sm:px-6 lg:px-10">
        <router-link to="/" class="inline-flex min-w-0 items-center gap-2.5 text-foreground no-underline">
          <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary font-serif text-lg font-semibold text-primary-foreground shadow-sm">S</span>
          <span class="truncate font-serif text-lg font-bold tracking-tight">SimpleBase</span>
        </router-link>
        <div class="flex shrink-0 items-center gap-2 sm:gap-5">
          <router-link to="/docs" class="hidden text-sm font-medium text-muted-foreground transition-colors hover:text-foreground sm:inline-flex">使用文档</router-link>
          <router-link :to="{ name: 'agents' }" class="inline-flex min-h-10 items-center justify-center gap-2 rounded-lg bg-primary px-3 text-sm font-semibold text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:px-4">
            控制台 <ArrowUpRightIcon aria-hidden="true" class="size-4" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10">
      <section class="mx-auto grid w-full max-w-7xl items-center gap-12 px-4 pb-20 pt-20 sm:px-6 sm:pt-28 lg:min-h-[660px] lg:grid-cols-[1.05fr_0.95fr] lg:gap-16 lg:px-10 lg:pb-28 lg:pt-24">
        <div class="max-w-xl">
          <div class="mb-7 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1.5 text-xs font-semibold tracking-[0.12em] text-primary uppercase">
            <span aria-hidden="true" class="size-1.5 rounded-full bg-primary" /> SimpleBase · WORKSPACE
          </div>
          <h1 class="font-serif text-[clamp(2.75rem,6vw,5.5rem)] leading-[1.09] font-semibold tracking-[-0.045em]">
            为 AI 数据和应用构建的<span class="block text-primary">一体化工作台</span>
          </h1>
          <p class="mt-7 max-w-lg text-base leading-8 text-muted-foreground sm:text-lg">
            管理数据库、对象存储与云函数，通过 LLM Gateway 接入模型并创建云助手，在一个控制台中构建 AI 数据和应用。
          </p>
          <div class="mt-9 flex flex-wrap items-center gap-3">
            <router-link :to="{ name: 'agents' }" class="inline-flex min-h-11 items-center justify-center gap-2 rounded-xl bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[0_12px_32px_-16px_var(--primary)] transition-colors hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
              进入控制台 <ArrowRightIcon aria-hidden="true" class="size-4" />
            </router-link>
            <router-link to="/docs" class="inline-flex min-h-11 items-center justify-center rounded-xl border border-border bg-card/80 px-6 text-sm font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
              查看文档
            </router-link>
          </div>
          <div class="mt-12 flex flex-wrap items-center gap-x-6 gap-y-3 border-t border-border/80 pt-5 text-xs text-muted-foreground">
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> 数据管理</span>
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> AI 构建</span>
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> 可观测</span>
          </div>
        </div>

        <aside aria-label="产品能力示意" class="relative mx-auto w-full max-w-[520px] lg:max-w-none">
          <div aria-hidden="true" class="absolute -inset-5 rounded-[2rem] bg-brand/10 blur-3xl" />
          <div class="relative overflow-hidden rounded-2xl border border-border bg-card/95 shadow-[0_24px_80px_-40px_var(--foreground)] dark:shadow-[0_24px_80px_-40px_black]">
            <div class="flex h-12 items-center justify-between border-b border-border bg-muted/40 px-5">
              <div class="flex items-center gap-2"><span aria-hidden="true" class="size-2 rounded-full bg-primary" /><span class="font-mono text-[11px] font-semibold tracking-widest text-muted-foreground">SIMPLEBASE / WORKSPACE</span></div>
              <span class="font-mono text-[10px] text-muted-foreground">OVERVIEW</span>
            </div>
            <div class="p-5 sm:p-7">
              <div class="flex items-center justify-between gap-3"><span class="text-sm font-semibold">工作台概览</span><span class="rounded-md border border-primary/25 bg-primary/10 px-2 py-1 font-mono text-[10px] text-primary">PRODUCT PREVIEW</span></div>
              <div class="mt-6 grid grid-cols-2 gap-3">
                <div v-for="item in previewItems" :key="item.label" class="rounded-xl border border-border bg-background/70 p-4">
                  <component :is="item.icon" aria-hidden="true" class="size-5 text-primary" />
                  <div class="mt-5 font-mono text-[10px] tracking-wider text-muted-foreground">{{ item.code }}</div>
                  <div class="mt-1 text-sm font-semibold">{{ item.label }}</div>
                </div>
              </div>
              <div class="mt-5 overflow-hidden rounded-xl border border-border bg-background/70">
                <div class="flex items-center justify-between border-b border-border px-4 py-3"><span class="font-mono text-[10px] tracking-wider text-muted-foreground">CAPABILITY MAP</span><span aria-hidden="true" class="size-1.5 rounded-full bg-primary" /></div>
                <div class="space-y-3 px-4 py-4">
                  <div v-for="(item, index) in previewItems" :key="item.code" class="flex items-center gap-3"><span class="w-5 font-mono text-[10px] text-primary">0{{ index + 1 }}</span><span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{{ item.label }}</span><span aria-hidden="true" class="h-px w-12 bg-primary/40 sm:w-20" /><span class="font-mono text-[10px] text-primary">READY</span></div>
                </div>
              </div>
            </div>
          </div>
          <div aria-hidden="true" class="absolute -bottom-4 -left-4 h-12 w-12 border-b border-l border-primary/60" />
          <div aria-hidden="true" class="absolute -right-4 -top-4 h-12 w-12 border-r border-t border-primary/60" />
        </aside>
      </section>

      <section class="border-t border-border bg-card/45 px-4 py-20 sm:px-6 lg:px-10" aria-labelledby="capabilities-heading">
        <div class="mx-auto max-w-7xl">
          <div class="mb-10 flex flex-wrap items-end justify-between gap-4">
            <div><p class="font-mono text-xs font-semibold tracking-[0.2em] text-primary uppercase">BUILT FOR YOUR WORKFLOW</p><h2 id="capabilities-heading" class="mt-3 font-serif text-3xl font-semibold tracking-tight sm:text-4xl">从数据管理到 AI 应用构建</h2></div>
            <router-link to="/docs" class="inline-flex min-h-10 items-center gap-1.5 text-sm font-medium text-primary hover:underline">浏览使用文档 <ArrowUpRightIcon aria-hidden="true" class="size-4" /></router-link>
          </div>
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <article v-for="item in capabilities" :key="item.title" class="group rounded-2xl border border-border bg-card p-6 transition-colors hover:border-primary/40">
              <div class="flex size-11 items-center justify-center rounded-xl border border-primary/20 bg-primary/10 text-primary"><component :is="item.icon" aria-hidden="true" class="size-5" /></div>
              <h3 class="mt-7 text-base font-semibold">{{ item.title }}</h3>
              <p class="mt-3 text-sm leading-7 text-muted-foreground">{{ item.description }}</p>
            </article>
          </div>
        </div>
      </section>
    </main>

    <footer class="relative z-10 border-t border-border px-4 py-7 sm:px-6 lg:px-10">
      <div class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 text-sm text-muted-foreground"><span class="font-serif font-semibold text-foreground">SimpleBase</span><div class="flex items-center gap-6"><router-link to="/docs" class="hover:text-foreground">使用文档</router-link><a href="https://github.com/linkxzhou/SimpleBase" target="_blank" rel="noopener noreferrer" class="hover:text-foreground">GitHub</a></div></div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ArrowRightIcon, ArrowUpRightIcon, BotIcon, CloudUploadIcon, CodeIcon, DatabaseIcon } from '@lucide/vue'

const previewItems = [
  { label: '数据库', code: 'DATABASE', icon: DatabaseIcon },
  { label: '对象存储', code: 'STORAGE', icon: CloudUploadIcon },
  { label: '云函数', code: 'FUNCTIONS', icon: CodeIcon },
  { label: '云助手', code: 'AGENTS', icon: BotIcon }
]

const capabilities = [
  { title: '数据库管理', description: '统一管理数据库与数据集合，让查询和日常维护保持清晰。', icon: DatabaseIcon },
  { title: '对象存储', description: '在工作台中管理项目文件与对象，轻松查看和组织资源。', icon: CloudUploadIcon },
  { title: '云函数', description: '编写、发布并管理云函数，将应用逻辑与数据连接起来。', icon: CodeIcon },
  { title: '云助手', description: '基于项目数据创建云助手，借助模型开展对话与只读查询。', icon: BotIcon }
]
</script>
