<template>
  <div class="navbar">
    <div class="nav-leading">
      <div class="logo">
        <a @click="gotoSystemClient">
          <img src="../../assets/logo.png">
        </a>
      </div>
      <div class="title">GoEdge</div>
    </div>
    <div class="tags-view-container"></div>
    <el-menu :default-active="activeIndex" router mode="horizontal" @select="handleSelect" class="right-menu"
      active-text-color="#1890ff">
      <el-menu-item index="/homepage">首页</el-menu-item>
      <el-menu-item index="/applications">应用列表</el-menu-item>
      <el-menu-item :index="appIndex">控制台</el-menu-item>
      <el-menu-item index="/document">使用文档</el-menu-item>
      <el-submenu index="/language">
        <template slot="title" class="user-name">中文</template>
        <el-menu-item index="/language/cn">中文</el-menu-item>
        <el-menu-item index="/language/en">English</el-menu-item>
      </el-submenu>
      <el-submenu index="" text-color="#1890ff">
        <template slot="title" class="user-name" v-if="isLogin">{{ name }}</template>
        <template slot="title" class="user-name" v-else>未登录用户</template>
        <el-menu-item index="/logout" @click.native="logout" v-if="isLogin">退出登录</el-menu-item>
        <el-menu-item index="/login" @click.native="logout" v-else>跳转登录</el-menu-item>
        <el-menu-item index="/user/github" v-if="isLogin"><a href="https://github.com/labring/laf/"
            target="_blank">GitHub地址</a>
        </el-menu-item>
        <el-menu-item index="/user/theme" v-if="isLogin">主题色
          <el-switch v-model="themeColor" style="margin: 10px;" active-color="#13ce66" inactive-color="#ff4949">
          </el-switch>
        </el-menu-item>
      </el-submenu>
    </el-menu>
  </div>
</template>

<script>
import Screenfull from '@/components/Screenfull'
import { openSystemClient } from '@/api/console'
import { getCurrentAppid } from '@/api/application'

export default {
  components: {
    Screenfull
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
    themeColor: {
      type: Boolean,
      default: true
    },
  },
  computed: {
    name() {
      const profile = this.$store.state.user.user_profile
      return profile?.username || profile?.name
    },
    isLogin() {
      return this.$store.state.user.user_profile ? true : false
    },
    appIndex() {
      let appid = getCurrentAppid()
      return appid ? `/app/${appid}/dashboard/index` : "/applications"
    },
    activeIndex() {
      if (this.$route.path.indexOf("/applications") >= 0) {
        return "/applications"
      } else if (this.$route.path.indexOf("/document") >= 0) {
        return "/document"
      } else if (this.$route.path.indexOf("/homepage") >= 0) {
        return "/homepage"
      } else {
        let appid = getCurrentAppid()
        return appid ? `/app/${appid}/dashboard/index` : "/applications"
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
    handleSelect(path, pathList) {
      this.$router.replace({ path: path })
    }
  }
}
</script>

<style lang="scss" scoped>
.navbar {
  height: 60px;
  display: flex;
  background: #fff;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .12), 0 0 3px 0 rgba(0, 0, 0, .04);
  align-items: center;

  .nav-leading {
    min-width: 140px;
    max-width: 210px;
    line-height: 60px;
    justify-content: flex-start;
    display: flex;
    align-items: center;
    padding: 0 10px;
    box-sizing: border-box;

    .logo {
      width: 40px;
      height: 40px;
      margin: 0 0 0 10px;

      img {
        width: 30px;
        height: 30px;
      }
    }

    .title {
      margin-left: 4px;
      font-size: 15px;
      font-weight: bold;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .tags-view-container {
    width: 100%;
    display: flex;
  }

  .right-menu {
    float: right;
    height: 100%;
    display: flex;
    font-weight: bold;
  }
}
</style>
