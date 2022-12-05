import store from '@/store'

const { body } = document
const WIDTH = 450 // refer to Bootstrap's responsive design

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
    // use $_ for mixins properties
    // https://vuejs.org/v2/style-guide/index.html#Private-property-names-essential
    $_isMobile() {
      const rect = body.getBoundingClientRect()
      return rect.width - 1 < WIDTH
    },
    $_resizeHandler() {
      if (!document.hidden) {
        const isMobile = this.$_isMobile()
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
