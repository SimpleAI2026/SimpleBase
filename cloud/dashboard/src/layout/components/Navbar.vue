<template>
  <div class="navbar">
    <div class="nav-leading">
      <div class="logo">
        <a @click="gotoSystemClient">
          <img src="../../assets/logo.png">
        </a>
      </div>
      <div class="title">{{ title }}</div>
    </div>
    <div class="sep" />
    <div class="tags-view-container">
      <!-- <tags-view v-if="!hideTags" /> -->
    </div>
    <el-menu :default-active="activeIndex" router mode="horizontal" @select="handleSelect" class="right-menu">
      <el-menu-item index="/index">首页</el-menu-item>
      <el-menu-item index="/applications">应用列表</el-menu-item>
      <el-menu-item index="/app/demo/index">控制台</el-menu-item>
      <el-submenu index="/">
        <template slot="title" class="user-name">{{ name }}</template>
        <el-menu-item index="/"><a href="https://github.com/labring/laf/" target="_blank">GitHub地址</a></el-menu-item>
        <el-menu-item index="/">切换主题</el-menu-item>
        <el-menu-item index="/" @click.native="logout">退出登录</el-menu-item>
      </el-submenu>
    </el-menu>
  </div>
</template>

<script>
import Screenfull from '@/components/Screenfull'
import TagsView from './TagsView'
import { openSystemClient } from '@/api/console'
export default {
  components: {
    Screenfull, TagsView
  },
  props: {
    hideTags: {
      type: Boolean,
      default: false
    },
    title: {
      type: String,
      default: 'GoEdge'
    },
  },
  computed: {
    name() {
      const profile = this.$store.state.user.user_profile
      return profile?.username || profile?.name
    },
    activeIndex() {
      if (this.$route.path.indexOf("/applications") >= 0) {
        return "/applications"
      } else if (this.$route.path.indexOf("/app/") >= 0) {
        return "/app/demo/index"
      } else {
        return "/index"
      }
    }
  },
  methods: {
    async logout() {
      await this.$store.dispatch('user/logout')
    },
    gotoSystemClient() {
      openSystemClient()
    },
    handleSelect(key, keyPath) {
    }
  }
}
</script>

<style lang="scss" scoped>
.navbar {
  height: 60px;
  display: flex;
  background: #fff;
  // border-bottom: 1px solid #d8dce5;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .12), 0 0 3px 0 rgba(0, 0, 0, .04);
  align-items: center;

  .nav-leading {
    width: 210px;
    min-width: 210px;
    line-height: 60px;
    justify-content: flex-start;
    display: flex;
    align-items: center;
    padding: 0 10px;
    box-sizing: border-box;

    .logo {
      width: 120px;
      height: 40px;

      img {
        width: 120px;
        height: 40px;
      }
    }

    .title {
      margin-left: 4px;
      font-size: 14px;
      font-weight: bold;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .sep {
    height: 60px;
    border-right: 1px solid rgba(196, 196, 196, 0.15);
  }

  .tags-view-container {
    width: 100%;
    padding: 0 10px;
    display: flex;
  }

  .right-menu {
    float: right;
    height: 100%;
    display: flex;
  }
}
</style>
