import store from '@/store'
import { __isMobile } from '@/utils/index'

export default {
  watch: {
    $route(route) { }
  },
  beforeMount() {
    window.addEventListener('resize', this.$_resizeHandler)
  },
  beforeDestroy() {
    window.removeEventListener('resize', this.$_resizeHandler)
  },
  mounted() {
    this.$_resizeHandler()
  },
  methods: {
    $_resizeHandler() {
      if (!document.hidden) {
        const isMobile = __isMobile()
        store.dispatch('settings/toggleDevice', isMobile ? 'mobile' : 'desktop')
        if (isMobile) {
          store.dispatch('settings/closeSideBar', { withoutAnimation: true })
          document.getElementsByTagName("body")[0].style.setProperty("--sideBarWidth", "50px");
        } else {
          store.dispatch('settings/toggleSideBar', { withoutAnimation: true })
          document.getElementsByTagName("body")[0].style.setProperty("--sideBarWidth", "210px");
        }
      }
    }
  }
}
