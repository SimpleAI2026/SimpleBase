<template>
  <div class="flex min-h-svh flex-col bg-background">
    <header class="sticky top-0 z-20 flex h-[var(--header-height)] shrink-0 items-center justify-between border-b bg-background/90 px-3 backdrop-blur-xl md:px-5">
      <div class="flex items-center gap-2.5">
        <router-link to="/" class="inline-flex items-center gap-2.5 text-foreground no-underline hover:text-primary" title="返回控制台">
          <span class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary font-serif text-base font-semibold text-primary-foreground">S</span>
          <span class="hidden font-serif text-base font-bold sm:inline">SimpleBase</span>
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
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" class="size-4">
              <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
            </svg>
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
</script>
