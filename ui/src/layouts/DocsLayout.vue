<template>
  <div class="flex min-h-svh flex-col bg-background">
    <header class="sticky top-0 z-20 flex h-[var(--header-height)] shrink-0 items-center justify-between border-b bg-background/90 px-3 backdrop-blur-xl md:px-5">
      <div class="flex items-center gap-2.5">
        <router-link to="/" class="inline-flex items-center gap-2.5 text-foreground no-underline hover:text-primary" title="返回控制台">
          <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary font-serif text-[17px] font-semibold text-primary-foreground">S</span>
          <span class="hidden font-serif text-[17px] font-bold sm:inline">SimpleBase</span>
        </router-link>
        <span class="text-muted-foreground" aria-hidden="true">/</span>
        <router-link to="/docs" class="text-sm font-semibold text-foreground no-underline hover:text-primary">使用文档</router-link>
      </div>
      <div class="flex items-center gap-2.5">
        <Button variant="ghost" size="sm" as-child>
          <router-link to="/">返回控制台</router-link>
        </Button>
        <Button variant="ghost" size="sm" as-child>
          <a href="https://github.com/linkxzhou/SimpleBase" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1">
            <GithubMark />
            <span class="hidden sm:inline">GitHub</span>
          </a>
        </Button>
      </div>
    </header>
    <main class="min-h-0 w-full flex-1">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { Button } from '@/components/ui/button'
import GithubMark from '../components/icons/GithubMark.vue'
</script>
