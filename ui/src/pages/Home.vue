<template>
  <div class="min-h-svh bg-background text-foreground">
    <header class="border-b border-border">
      <nav :aria-label="t('pages.home.mainNav')" class="mx-auto flex h-14 w-full max-w-7xl items-center gap-3 px-5 sm:h-18 sm:px-8 lg:px-12">
        <router-link to="/" class="no-underline">
          <BrandLogo size="lg" />
        </router-link>
      </nav>
    </header>

    <main>
      <!-- 首屏：左 5 栏只讲一句定位 + 两个动作；右 7 栏是同一个应用的连续演示（§12.5） -->
      <section class="mx-auto grid w-full max-w-7xl items-center gap-10 px-5 pt-10 pb-12 sm:px-8 sm:pt-18 sm:pb-16 lg:min-h-[min(calc(100svh-4.5rem),50rem)] lg:grid-cols-12 lg:gap-14 lg:px-12 lg:py-16">
        <div class="max-w-[30rem] lg:col-span-5">
          <h1 class="m-0 text-[2.25rem] leading-[1.18] font-medium text-foreground sm:text-5xl sm:leading-[1.14] lg:text-[3.75rem] lg:leading-[1.12] lg:tracking-[-0.02em]">
            {{ t('pages.home.heroTitlePre') }}<span class="block sm:whitespace-nowrap">{{ t('pages.home.heroTitleHighlight') }}</span>
          </h1>
          <p class="mt-6 mb-0 text-base leading-[1.625rem] text-text-secondary sm:text-lg sm:leading-[1.875rem]">
            {{ t('pages.home.heroDescription') }}
          </p>
          <div class="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-5">
            <router-link :to="{ name: 'agents' }" :class="[primaryBtn, 'inline-flex h-11 px-5']">
              {{ t('pages.home.enterConsole') }} <ArrowRightIcon aria-hidden="true" class="size-3.5" />
            </router-link>
            <router-link :to="shipGuide" class="inline-flex h-11 items-center gap-1.5 self-start text-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:decoration-foreground sm:self-auto">
              {{ t('pages.home.viewDocs') }} <ArrowUpRightIcon aria-hidden="true" class="size-3.5" />
            </router-link>
          </div>
        </div>

        <aside :aria-label="t('pages.home.previewAria')" class="w-full min-w-0 lg:col-span-7">
          <HomeDemo />
        </aside>
      </section>

      <!-- 下半页：非对称、证据优先（§12.7） -->
      <section class="border-t border-border" aria-labelledby="capabilities-heading">
        <div class="mx-auto flex max-w-7xl flex-col gap-6 px-5 py-16 sm:px-8 sm:py-20 lg:px-12 lg:py-24">
          <h2 id="capabilities-heading" class="m-0 mb-6 max-w-2xl sm:mb-10 text-[1.625rem] leading-[1.3] font-medium sm:text-4xl sm:leading-[1.25]">{{ t('pages.home.capabilitiesTitle') }}</h2>

          <!-- 01 AI 工作台：左 40% 解释，右 60% 静态证据 -->
          <article class="grid grid-cols-1 items-center gap-6 rounded-xl border border-border bg-card p-6 sm:p-8 lg:grid-cols-12 lg:gap-8" data-capability="agent">
            <div class="flex flex-col gap-3 lg:col-span-5">
              <span class="sb-mono text-xs text-primary">01</span>
              <h3 class="m-0 text-xl leading-7 font-medium">{{ t('pages.home.capabilityAgentsTitle') }}</h3>
              <p class="m-0 text-sm leading-6 text-text-secondary">{{ t('pages.home.capabilityAgentsDesc') }}</p>
            </div>
            <div class="overflow-hidden rounded-lg bg-muted lg:col-span-7">
              <div class="grid grid-cols-1 sm:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)]">
                <div v-for="(col, ci) in agentEvidence" :key="col.label" class="flex flex-col gap-2 border-border p-4" :class="ci > 0 && 'border-t sm:border-t-0 sm:border-l'">
                  <span class="text-xs text-muted-foreground">{{ col.label }}</span>
                  <span v-for="line in col.lines" :key="line" :class="ci === 0 ? 'text-sm leading-6 text-foreground' : 'sb-mono text-xs leading-5 text-text-secondary'">{{ line }}</span>
                </div>
              </div>
            </div>
          </article>

          <!-- 02 / 03 等高：网格拉伸，03 的底座图贴底 -->
          <div class="grid grid-cols-1 items-stretch gap-6 lg:grid-cols-12">
            <!-- 02 Demo to online：本地与云端同名资源对照 -->
            <article class="flex flex-col gap-5 rounded-xl border border-border bg-card p-6 sm:p-8 lg:col-span-7" data-capability="ship">
              <div class="flex flex-col gap-3">
                <span class="sb-mono text-xs text-primary">02</span>
                <h3 class="m-0 text-xl leading-7 font-medium">{{ t('pages.home.capabilityShipTitle') }}</h3>
                <p class="m-0 text-sm leading-6 text-text-secondary">{{ t('pages.home.capabilityShipDesc') }}</p>
              </div>
              <div class="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3 sm:gap-4">
                <div v-for="(side, si) in shipSides" :key="side.label" class="flex min-w-0 flex-col gap-1 rounded-lg bg-muted p-3" :class="si === 1 && 'col-start-3'">
                  <span class="text-xs text-muted-foreground">{{ side.label }}</span>
                  <span v-for="r in shipResources" :key="r" class="sb-mono flex items-center gap-1.5 text-xs text-foreground">
                    <CheckIcon v-if="si === 1" aria-hidden="true" class="size-3 shrink-0 text-success" />
                    <span class="min-w-0 break-all">{{ r }}</span>
                  </span>
                </div>
                <ArrowRightIcon aria-hidden="true" class="col-start-2 row-start-1 size-4 text-muted-foreground" />
              </div>
              <pre class="sb-mono m-0 overflow-x-auto rounded-lg bg-muted px-3 py-2 text-xs leading-5 text-foreground"><span class="text-muted-foreground">$ </span>simplebase dev apply --dir . --yes</pre>
            </article>

            <!-- 03 共享存储底座：架构说明，前提就近 -->
            <article class="flex flex-col gap-5 rounded-xl border border-border bg-card p-6 sm:p-8 lg:col-span-5" data-capability="cost">
              <div class="flex flex-col gap-3">
                <span class="sb-mono text-xs text-primary">03</span>
                <h3 class="m-0 text-xl leading-7 font-medium">{{ t('pages.home.capabilityCostTitle') }}</h3>
                <p class="m-0 text-sm leading-6 text-text-secondary">{{ t('pages.home.capabilityCostDesc') }}</p>
              </div>
              <!-- 共享底座：五个模块等宽站在同一条通宽底座上（不是先后流程） -->
              <!-- flex-1：图示填满卡片剩余高度，与 02 卡片的对照区对齐，不留中部空洞 -->
              <div class="flex min-h-36 flex-1 flex-col gap-1" data-foundation-diagram>
                <div class="grid flex-1 grid-cols-5 gap-1">
                  <span v-for="m in baseline" :key="m" class="flex items-center justify-center rounded-t-md bg-muted px-1 text-center text-xs leading-4 text-text-secondary">{{ m }}</span>
                </div>
                <span class="rounded-b-md bg-primary px-3 py-2 text-center text-sm font-medium text-primary-foreground">{{ t('pages.home.foundationLabel') }}</span>
              </div>
            </article>
          </div>
        </div>
      </section>
    </main>

    <footer class="border-t border-border">
      <div class="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-5 py-8 text-sm text-muted-foreground sm:px-8 lg:px-12">
        <span class="font-medium text-foreground">SimpleBase</span>
        <div class="flex items-center gap-6">
          <router-link to="/docs" class="inline-flex h-11 items-center transition-colors hover:text-foreground">{{ t('pages.home.docs') }}</router-link>
          <a href="https://github.com/linkxzhou/SimpleBase" target="_blank" rel="noopener noreferrer" class="inline-flex h-11 items-center transition-colors hover:text-foreground">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRightIcon, ArrowUpRightIcon, CheckIcon } from '@lucide/vue'
import BrandLogo from '@/components/BrandLogo.vue'
import HomeDemo from '@/components/home/HomeDemo.vue'

const { t } = useI18n()

/** 首屏主按钮：与控制台同一主色。 */
const primaryBtn =
  'shrink-0 items-center justify-center gap-2 self-start rounded-lg bg-primary text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:self-auto'

/** 「上线指南」指向真实的本地开发 / 云端重放文档。 */
const shipGuide = '/docs/getting-started/local-cloud-collab'

const baseline = computed(() => [
  t('pages.home.demo.modDatabase'),
  t('pages.home.demo.modKv'),
  t('pages.home.demo.modFunctions'),
  t('pages.home.demo.modTriggers'),
  t('pages.home.demo.modCron')
])

const agentEvidence = computed(() => [
  { label: t('pages.home.evidenceRequest'), lines: [t('pages.home.demo.chatAsk')] },
  { label: t('pages.home.evidenceTools'), lines: ['sql exec', 'function create', 'cron create'] },
  { label: t('pages.home.evidenceResources'), lines: ['orders', 'daily_summary', 'nightly · 02:00'] }
])

const shipSides = computed(() => [{ label: t('pages.home.demo.local') }, { label: t('pages.home.demo.cloud') }])
const shipResources = ['shop', '001_orders.sql', 'daily_summary', 'nightly']
</script>
