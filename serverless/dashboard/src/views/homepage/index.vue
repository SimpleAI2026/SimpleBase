<template>
  <div>
    <div class="banner">
      <div class="banner-desc">
        <h1>
          Serverless平台
        </h1>
        <p>
          GoEdge为Golang，Webassembly开发者提供一站式开发的云平台，开箱即用。
        </p>
      </div>
    </div>
    <div class="jumbotron" ref="indexMainImg">
      <img src="../../assets/homepage/homepage.png" alt="">
    </div>
  </div>
</template>
<script>
export default {
  methods: {
    handleScroll(index) {
      const ele = this.$refs.indexMainImg;
      const rect = ele.getBoundingClientRect();
      const eleHeight = ele.clientHeight + 55;
      let calHeight = (180 - rect.top) * 2;
      if (calHeight < 0) calHeight = 0;
      if (calHeight > eleHeight) calHeight = eleHeight;
      this.mainImgOffset = calHeight;
    }
  },
  data() {
    return {
      lang: this.$route.meta.lang,
      mainImgOffset: 0
    };
  },
  beforeDestroy() {
    window.removeEventListener('scroll', this.throttledHandleScroll);
  },
  mounted() {
    window.addEventListener('scroll', this.throttledHandleScroll);
  }
}
</script>

<style lang="scss" scoped>
.banner {
  text-align: center;
}

.banner-desc {
  padding-top: 50px;

  h1 {
    font-size: 34px;
    margin: 0;
    line-height: 48px;
    color: #555;
  }

  p {
    font-size: 18px;
    line-height: 28px;
    color: #888;
    margin: 10px 0 5px;
  }
}

.jumbotron {
  max-width: 890px;
  min-width: 200px;
  margin: 10px auto;
  position: relative;

  img {
    width: 100%;
  }
}

@media (max-width: 1140px) {
  .banner .container {
    width: 100%;
    box-sizing: border-box;
  }

  .banner img {
    right: 0;
  }
}

@media (max-width: 1000px) {
  .banner .container {
    img {
      display: none;
    }
  }
}

@media (max-width: 768px) {
  .banner-stars {
    display: none;
  }

  .banner-desc {
    #line2 {
      display: none;
    }

    h2 {
      font-size: 32px;
    }

    p {
      width: auto;
    }
  }
}
</style>