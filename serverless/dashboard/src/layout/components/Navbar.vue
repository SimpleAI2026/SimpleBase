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
    <div class="right-menu">
      <el-menu :default-active="activeIndex" router mode="horizontal" @select="handleSelect"
        active-text-color="#1890ff">
        <el-menu-item index="/homepage" v-if="!isMobile">首页</el-menu-item>
        <el-menu-item index="/applications" v-if="!isMobile">应用列表</el-menu-item>
        <el-menu-item :index="appIndex" v-if="!isMobile">控制台</el-menu-item>
        <el-menu-item index="/document" v-if="!isMobile">使用文档</el-menu-item>
        <el-submenu index="/language" v-if="!isMobile">
          <template slot="title">中文</template>
          <el-menu-item>中文</el-menu-item>
          <el-menu-item>English</el-menu-item>
        </el-submenu>
        <el-submenu index="/language" v-if="isMobile">
          <template slot="title">首页</template>
          <el-menu-item index="/applications">应用列表</el-menu-item>
          <el-menu-item :index="appIndex">控制台</el-menu-item>
          <el-menu-item index="/document">使用文档</el-menu-item>
        </el-submenu>
        <el-submenu index="" text-color="#1890ff">
          <template slot="title" v-if="isLogin">{{ name }}</template>
          <template slot="title" v-else>未登录用户</template>
          <el-menu-item @click.native="logout" v-if="isLogin">退出登录</el-menu-item>
          <el-menu-item v-else>跳转登录</el-menu-item>
          <el-menu-item><a href="https://github.com/labring/laf/" target="_blank">GitHub地址</a>
          </el-menu-item>
        </el-submenu>
      </el-menu>
    </div>
  </div>
</template>

<script>
import variables from '@/styles/element-variables.scss'
import { getCurrentAppid } from '@/api/index'
import { __isMobile } from '@/utils/index'

export default {
  components: {},
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
  data() {
    return {
      themeColor: true,
      isMobile: __isMobile(),
    }
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
    },
  },
  methods: {
    async logout() {
      await this.$store.dispatch('user/logout')
      this.$router.replace({ path: "/login" })
    },
    gotoSystemClient() {
      window.open('/', '_self')
    },
    handleSelect(path, pathList) { }
  }
}
</script>

<style lang="scss" scoped>
.navbar {
  display: flex;
  background: #fff;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, .12), 0 0 3px 0 rgba(0, 0, 0, .04);
  align-items: center;
  justify-content: space-between;

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
      color: #4f5959;
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
