<template>
  <a-config-provider :theme="theme">
    <DefaultLayout />
  </a-config-provider>
</template>
<script setup lang="ts">
import DefaultLayout from './layouts/DefaultLayout.vue'

// antd v4 通过 design token 统一主题色，与 theme.css 的 --sb-* 色板保持一致
const theme = {
  token: {
    colorPrimary: '#d97757',
    colorInfo: '#d97757',
    colorSuccess: '#3f8a5a',
    colorWarning: '#c99a2c',
    colorError: '#c0452f',
    colorBgLayout: '#f5f4ef',
    colorText: '#1f1e1d',
    colorTextSecondary: '#706f6a',
    borderRadius: 10
  }
}
</script>
