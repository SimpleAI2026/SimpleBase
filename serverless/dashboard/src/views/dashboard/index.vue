<template>
  <div class="application-container">
    <el-row>
      <el-col :span="elCol">
        <el-card class="chart">
          <div slot="header" class="clearfix">
            <span>详情</span>
          </div>
          <div class="text item">
            <div class="label">应用名：{{ app.name }}</div>
            <div class="label">应用ID：{{ app.appid }}</div>
            <div class="label">服务地址：{{ getAppUrl() }}</div>
            <div class="label">使用文档：<a style="color: blue" target="_blank"
                href="https://docs.lafyun.com/">https://docs.lafyun.com/</a></div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="elCol">
        <el-card class="chart">
          <v-chart ref="runTimes_creditChart" :auto-resize="true" :option="option_req"
            :style="{ width: '100%', height: '400px' }" />
        </el-card>
      </el-col>
    </el-row>
    <el-row>
      <el-col :span="elCol">
        <el-card class="chart">
          <v-chart ref="runTimes_creditChart" :auto-resize="true" :option="option_error"
            :style="{ width: '100%', height: '400px' }" />
        </el-card>
      </el-col>
      <el-col :span="elCol">
        <el-card class="chart">
          <v-chart ref="runTimes_creditChart" :auto-resize="true" :option="option_location"
            :style="{ width: '100%', height: '400px' }" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script>
import { getAppAccessUrl } from '@/api/application'
import { use } from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { PieChart, LineChart, BarChart } from 'echarts/charts';
import {
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
} from 'echarts/components';
import VChart from 'vue-echarts';
import { __isMobile } from '@/utils/index'

use([
  CanvasRenderer,
  PieChart,
  TitleComponent,
  TooltipComponent,
  LegendComponent,
  GridComponent,
  LineChart,
  BarChart,
]);

export default {
  name: 'Dashboard',
  components: {
    VChart
  },
  data() {
    return {
      option_req: {
        title: {
          text: '请求总量',
          left: 'center',
        },
        xAxis: {
          type: 'category',
          data: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
        },
        yAxis: {
          type: 'value'
        },
        series: [
          {
            data: [150, 230, 224, 218, 135, 147, 260],
            type: 'line',
            smooth: true
          }
        ]
      },
      option_error: {
        title: {
          text: '请求错误量',
          left: 'center',
        },
        tooltip: {
          trigger: 'item',
          formatter: '{a} <br/>{b} : {c} ({d}%)',
        },
        xAxis: {
          type: 'category',
          data: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
        },
        yAxis: {
          type: 'value'
        },
        series: [
          {
            data: [120, 200, 150, 80, 70, 110, 130],
            type: 'bar'
          }
        ]
      },
      option_location: {
        title: {
          text: '地域分布',
          left: 'center',
        },
        tooltip: {
          trigger: 'item',
          formatter: '{a} <br/>{b} : {c} ({d}%)',
        },
        series: [
          {
            name: 'Traffic Sources',
            type: 'pie',
            radius: '55%',
            center: ['50%', '60%'],
            data: [
              { value: 335, name: 'Direct' },
              { value: 310, name: 'Email' },
              { value: 234, name: 'Ad Networks' },
              { value: 135, name: 'Video Ads' },
              { value: 1548, name: 'Search Engines' },
            ],
            emphasis: {
              itemStyle: {
                shadowBlur: 10,
                shadowOffsetX: 0,
                shadowColor: 'rgba(0, 0, 0, 0.5)',
              },
            },
          },
        ],
      }
    }
  },
  computed: {
    app() {
      return this.$store.state.app?.application
    },
    elCol() {
      return __isMobile() ? 24 : 12
    },
    charStyle() {
      return __isMobile() ? "{ width: '350px', height: '400px' }" : "{ height: '400px' }"
    },
  },
  mounted() {
  },
  async created() { },
  methods: {
    getAppUrl() {
      return getAppAccessUrl()
    }
  }
}
</script>

<style scoped>
.application-container {
  padding-top: 15px;
}

.clearfix span {
  font-size: 18px;
  font-weight: bold;
}

.label {
  color: rgb(82, 80, 80);
  font-size: 16px;
  margin: 10px;
}

.row .content {
  color: black;
  padding-left: 10px;
}

.chart {
  height: 400px;
  margin: 5px;
}
</style>
