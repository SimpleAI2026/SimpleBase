<template>
  <div class="docs-layout">
    <header class="docs-topbar">
      <div class="docs-topbar-left">
        <router-link to="/" class="docs-brand" title="返回控制台">
          <span class="docs-brand-logo">S</span>
          <span class="docs-brand-text">SimpleBase</span>
        </router-link>
        <span class="docs-topbar-sep" aria-hidden="true">/</span>
        <router-link to="/docs" class="docs-topbar-title">使用文档</router-link>
      </div>
      <div class="docs-topbar-right">
        <router-link to="/" class="docs-topbar-link">返回控制台</router-link>
        <a
          href="https://github.com/linkxzhou/SimpleBase"
          target="_blank"
          rel="noopener noreferrer"
          class="docs-topbar-link"
        >
          <GithubOutlined />
          <span class="docs-topbar-link-text">GitHub</span>
        </a>
      </div>
    </header>
    <main class="docs-shell">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { GithubOutlined } from '@ant-design/icons-vue'
</script>

<style scoped>
.docs-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--sb-bg, #f5f4ef);
}
.docs-topbar {
  height: var(--sb-header-height, 56px);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  background: rgba(250, 249, 245, 0.92);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid var(--sb-border-soft, rgba(31, 30, 29, 0.08));
  position: sticky;
  top: 0;
  z-index: 20;
}
.docs-topbar-left,
.docs-topbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
.docs-brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  color: var(--sb-text, #1f1e1d);
}
.docs-brand-logo {
  width: 32px;
  height: 32px;
  border-radius: var(--sb-radius-sm, 8px);
  background: var(--sb-primary, #d97757);
  color: #fff;
  font-family: var(--sb-font-serif, Georgia, serif);
  font-weight: 600;
  font-size: 17px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.docs-brand-text {
  font-family: var(--sb-font-serif, Georgia, serif);
  font-size: 17px;
  font-weight: 700;
}
.docs-topbar-sep {
  color: var(--sb-text-muted, #9b9a94);
}
.docs-topbar-title {
  color: var(--sb-text, #1f1e1d);
  text-decoration: none;
  font-weight: 600;
  font-size: 14px;
}
.docs-topbar-title:hover,
.docs-brand:hover {
  color: var(--sb-primary, #d97757);
}
.docs-topbar-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--sb-text-secondary, #706f6a);
  text-decoration: none;
  font-size: 13px;
}
.docs-topbar-link:hover {
  color: var(--sb-primary, #d97757);
}
.docs-shell {
  flex: 1;
  min-height: 0;
  width: 100%;
}

@media (max-width: 768px) {
  .docs-topbar {
    padding: 0 12px;
  }
  .docs-brand-text,
  .docs-topbar-link-text {
    display: none;
  }
}
</style>
