<template>
  <div class="relative min-h-svh overflow-hidden bg-background text-foreground">
    <div aria-hidden="true" class="pointer-events-none absolute inset-x-0 top-0 h-[760px] bg-[radial-gradient(ellipse_at_70%_20%,color-mix(in_srgb,var(--brand)_13%,transparent),transparent_57%)]" />
    <div aria-hidden="true" class="pointer-events-none absolute inset-x-0 top-0 h-[850px] opacity-35 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] bg-[size:52px_52px] [mask-image:linear-gradient(to_bottom,black,transparent)]" />

    <header class="relative z-10 border-b border-border/70 bg-background/75 backdrop-blur-xl">
      <nav :aria-label="t('pages.home.mainNav')" class="mx-auto flex h-16 w-full max-w-7xl items-center justify-between gap-3 px-4 sm:px-6 lg:px-10">
        <router-link to="/" class="inline-flex min-w-0 items-center gap-2.5 text-foreground no-underline">
          <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary font-serif text-lg font-semibold text-primary-foreground shadow-sm">S</span>
          <span class="truncate font-serif text-lg font-bold tracking-tight">SimpleBase</span>
        </router-link>
        <div class="flex shrink-0 items-center gap-2 sm:gap-5">
          <router-link to="/docs" class="hidden text-sm font-medium text-muted-foreground transition-colors hover:text-foreground sm:inline-flex">{{ t('pages.home.docs') }}</router-link>
          <router-link :to="{ name: 'agents' }" class="inline-flex min-h-10 items-center justify-center gap-2 rounded-lg bg-primary px-3 text-sm font-semibold text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:px-4">
            {{ t('pages.home.console') }} <ArrowUpRightIcon aria-hidden="true" class="size-4" />
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
          <h1 class="font-serif text-[clamp(2.25rem,5vw,4.5rem)] leading-[1.09] font-semibold tracking-[-0.045em]">
            {{ t('pages.home.heroTitlePre') }}<span class="block text-[clamp(2.25rem,4vw,3.5rem)] text-primary sm:whitespace-nowrap">{{ t('pages.home.heroTitleHighlight') }}</span>
          </h1>
          <p class="mt-7 max-w-lg text-base leading-8 text-muted-foreground sm:text-lg">
            {{ t('pages.home.heroDescription') }}
          </p>
          <div class="mt-9 flex flex-wrap items-center gap-3">
            <router-link :to="{ name: 'agents' }" class="inline-flex min-h-11 items-center justify-center gap-2 rounded-xl bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-[0_12px_32px_-16px_var(--primary)] transition-colors hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
              {{ t('pages.home.enterConsole') }} <ArrowRightIcon aria-hidden="true" class="size-4" />
            </router-link>
            <router-link to="/docs" class="inline-flex min-h-11 items-center justify-center rounded-xl border border-border bg-card/80 px-6 text-sm font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
              {{ t('pages.home.viewDocs') }}
            </router-link>
          </div>
          <div class="mt-12 flex flex-wrap items-center gap-x-6 gap-y-3 border-t border-border/80 pt-5 text-xs text-muted-foreground">
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> {{ t('pages.home.tagData') }}</span>
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> {{ t('pages.home.tagAi') }}</span>
            <span class="inline-flex items-center gap-2"><span aria-hidden="true" class="size-1.5 rounded-full bg-brand" /> {{ t('pages.home.tagObservability') }}</span>
          </div>
        </div>

        <aside :aria-label="t('pages.home.previewAria')" class="relative mx-auto w-full max-w-[520px] lg:max-w-none">
          <div aria-hidden="true" class="absolute -inset-5 rounded-[2rem] bg-brand/10 blur-3xl" />
          <div class="relative overflow-hidden rounded-2xl border border-border bg-card/95 shadow-[0_24px_80px_-40px_var(--foreground)] dark:shadow-[0_24px_80px_-40px_black]">
            <div class="flex h-12 items-center justify-between border-b border-border bg-muted/40 px-5">
              <div class="flex items-center gap-2"><span aria-hidden="true" class="size-2 rounded-full bg-primary" /><span class="font-mono text-[11px] font-semibold tracking-widest text-muted-foreground">SIMPLEBASE / WORKSPACE</span></div>
              <span class="font-mono text-[10px] text-muted-foreground">OVERVIEW</span>
            </div>
            <div class="p-3 sm:p-7">
              <div class="flex items-center justify-between gap-3"><span class="text-sm font-semibold">{{ t('pages.home.previewTitle') }}</span><span class="hidden rounded-md border border-primary/25 bg-primary/10 px-2 py-1 font-mono text-[10px] text-primary sm:inline">PRODUCT PREVIEW</span></div>
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
            <div><p class="font-mono text-xs font-semibold tracking-[0.2em] text-primary uppercase">BUILT FOR YOUR WORKFLOW</p><h2 id="capabilities-heading" class="mt-3 font-serif text-3xl font-semibold tracking-tight sm:text-4xl">{{ t('pages.home.capabilitiesTitle') }}</h2></div>
            <router-link to="/docs" class="inline-flex min-h-10 items-center gap-1.5 text-sm font-medium text-primary hover:underline">{{ t('pages.home.browseDocs') }} <ArrowUpRightIcon aria-hidden="true" class="size-4" /></router-link>
          </div>
          <div class="grid gap-4 md:grid-cols-3">
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
      <div class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 text-sm text-muted-foreground"><span class="font-serif font-semibold text-foreground">SimpleBase</span><div class="flex items-center gap-6"><router-link to="/docs" class="hover:text-foreground">{{ t('pages.home.docs') }}</router-link><a href="https://github.com/linkxzhou/SimpleBase" target="_blank" rel="noopener noreferrer" class="hover:text-foreground">GitHub</a></div></div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRightIcon, ArrowUpRightIcon, BotIcon, CloudUploadIcon, CodeIcon, DatabaseIcon } from '@lucide/vue'

const { t } = useI18n()

const previewItems = computed(() => [
  { label: t('pages.home.previewAgents'), code: 'AGENTS', icon: BotIcon },
  { label: t('pages.home.previewDatabase'), code: 'DATABASE', icon: DatabaseIcon },
  { label: t('pages.home.previewStorage'), code: 'STORAGE', icon: CloudUploadIcon },
  { label: t('pages.home.previewFunctions'), code: 'FUNCTIONS', icon: CodeIcon }
])

const capabilities = computed(() => [
  {
    title: t('pages.home.capabilityAgentsTitle'),
    description: t('pages.home.capabilityAgentsDesc'),
    icon: BotIcon
  },
  {
    title: t('pages.home.capabilityStorageTitle'),
    description: t('pages.home.capabilityStorageDesc'),
    icon: DatabaseIcon
  },
  {
    title: t('pages.home.capabilityFunctionsTitle'),
    description: t('pages.home.capabilityFunctionsDesc'),
    icon: CloudUploadIcon
  }
])
</script>
