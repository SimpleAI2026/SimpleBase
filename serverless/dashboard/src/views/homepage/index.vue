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
    <div class="jumbotron">
      <img src="../../assets/homepage/homepage.png" alt="">
    </div>
    <div class="cards">
      <ul class="container">
        <li>
          <div class="card">
            <img src="../../assets/homepage/component.png" alt="">
            <h3>开发</h3>
            <p>支持Golang，Webassembly开发模式，提供存储，数据库等托管</p>
            <router-link active-class="active" to="" exact>查看详情</router-link>
          </div>
        </li>
        <li>
          <div class="card">
            <img src="../../assets/homepage/resource.png" alt="">
            <h3>文档</h3>
            <p>了解文档，快速组合或者开发业务功能</p>
            <router-link active-class="active" to="" exact>查看详情</router-link>
          </div>
        </li>
        <li>
          <div class="card">
            <img src="../../assets/homepage/chart.svg" alt="">
            <h3>图表</h3>
            <p>详细的图表，便捷的查看线上服务状态</p>
            <router-link active-class="active" to="" exact>查看详情</router-link>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>
<script>
export default {
  methods: {
  },
  data() {
    return {
      lang: this.$route.meta.lang,
      mainImgOffset: 0
    };
  },
  mounted() { }
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
    margin: 10px;
  }
}

.jumbotron {
  max-width: 1140px;
  min-width: 200px;
  margin: 10px auto;
  position: relative;

  img {
    width: 100%;
  }
}

.cards {
  margin: 0 auto 110px;
  max-width: 1140px;

  .container {
    padding: 0;
    margin: 0 -11px;
    width: auto;

    &::before,
    &::after {
      display: table;
      content: "";
    }

    &::after {
      clear: both;
    }
  }

  li {
    width: 33%;
    padding: 0 19px;
    box-sizing: border-box;
    float: left;
    list-style: none;
  }

  img {
    width: 160px;
    height: 120px;
  }
}

.card {
  height: 430px;
  width: 100%;
  background: #ffffff;
  border: 1px solid #eaeefb;
  border-radius: 5px;
  box-sizing: border-box;
  text-align: center;
  position: relative;
  transition: all .3s ease-in-out;
  bottom: 0;

  img {
    margin: 66px auto 60px;
  }

  h3 {
    margin: 0;
    font-size: 18px;
    color: #1f2f3d;
    font-weight: normal;
  }

  p {
    font-size: 14px;
    color: #99a9bf;
    padding: 0 25px;
    line-height: 20px;
  }

  a {
    height: 53px;
    line-height: 52px;
    font-size: 14px;
    color: #409EFF;
    text-align: center;
    border: 0;
    border-top: 1px solid #eaeefb;
    padding: 0;
    cursor: pointer;
    width: 100%;
    position: absolute;
    bottom: 0;
    left: 0;
    background-color: #fff;
    border-radius: 0 0 5px 5px;
    transition: all .3s;
    text-decoration: none;
    display: block;

    &:hover {
      color: #fff;
      background: #409EFF;
    }
  }

  &:hover {
    bottom: 6px;
    box-shadow: 0 6px 18px 0 rgba(232, 237, 250, 0.50);
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
  .cards {
    li {
      width: 80%;
      margin: 0 auto 20px;
      float: none;
    }

    .card {
      height: auto;
      padding-bottom: 54px;
    }
  }

  .banner-stars {
    display: none;
  }

  .banner-desc {
    #line2 {
      display: none;
    }

    h2 {
      font-size: 20px;
    }

    p {
      width: auto;
      font-size: 14px;
    }
  }
}
</style>