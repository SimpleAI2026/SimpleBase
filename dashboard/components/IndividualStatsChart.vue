<template >
  <div class="singleChart">
    <h4 class="chart-title">{{ chartConfig.NAME }}: {{ this.total }}</h4>
    <stats-chart-legend :chartConfig="chartConfig" :datacollection="datacollection">
    </stats-chart-legend>
    <line-chart :chart-data="datacollection" :options="{
      responsive: true,
      maintainAspectRatio: true,
      title: {
        display: false,
        text: '{{chartConfig.NAME}}'
      },
      legend: {
        display: false,
      },
      animation: {
        duration: 0 // turn off annoying bouncing animation
      },
      scales: {
        yAxes: [{
          stacked: '{{chartConfig.isStacked}}',
          ticks: {
            suggestedMax: 10
          }
        }]
      }
    }">
    </line-chart>
  </div>
</template>

<script>

import LineChart from './LineChart';
import StatsChartLegend from '../components/StatsChartLegend.vue';
import { eventBus } from '../main';
import { updateChart } from './graphUtilities';

export default {
  components: {
    LineChart,
    StatsChartLegend,
  },
  props: [
    'stats',
    'statshistory',
    'chartConfig',
    'appid', // this will be unset if this chart is for all apps
    'fns', // this will be unset if this chart is for all apps
  ],
  data() {
    return {
      datacollection: {},
      total: 0
    }
  },
  mounted() {
  },
  methods: {
  },
  created: function () {
    // handle "stats have been refreshed"
    eventBus.$on('statsRefreshed', (app) => {
      updateChart(this);
    });
  }
}

</script>
