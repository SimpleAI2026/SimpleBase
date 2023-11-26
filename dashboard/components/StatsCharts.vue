<template >
  <div class="row">
    <template v-if="showGeneralCharts">
      <individual-stats-chart :chartConfig="queuedChartConfig" :stats="stats"
        :statshistory="statshistory"></individual-stats-chart>
      <individual-stats-chart :chartConfig="runningChartConfig" :stats="stats"
        :statshistory="statshistory"></individual-stats-chart>
      <individual-stats-chart :chartConfig="completedChartConfig" :stats="stats"
        :statshistory="statshistory"></individual-stats-chart>
    </template>
    <template v-if="showAppCharts">
      <template v-if="stats.Apps && stats.Apps[appid]">
        <individual-stats-chart :chartConfig="startingChartConfig" :stats="stats" :statshistory="statshistory"
          :appid="appid" :fns="fns"></individual-stats-chart>
        <individual-stats-chart :chartConfig="waitingChartConfig" :stats="stats" :statshistory="statshistory"
          :appid="appid" :fns="fns"></individual-stats-chart>
        <individual-stats-chart :chartConfig="busyChartConfig" :stats="stats" :statshistory="statshistory" :appid="appid"
          :fns="fns"></individual-stats-chart>
        <individual-stats-chart :chartConfig="idlingChartConfig" :stats="stats" :statshistory="statshistory"
          :appid="appid" :fns="fns"></individual-stats-chart>
        <individual-stats-chart :chartConfig="pausedChartConfig" :stats="stats" :statshistory="statshistory"
          :appid="appid" :fns="fns"></individual-stats-chart>
      </template>
      <template v-else>
        <p>No statistics found for App. Possible reasons for this are:</p>
        <ul>
          <li>None of the App's functions have been run since the Fn server has started.</li>
          <li>The Fn server isn't configured to produce metrics for individual Apps.</li>
        </ul>
      </template>
    </template>
  </div>
</template>

<script>
import { chartConfig } from './graphUtilities';
import IndividualStatsChart from '../components/IndividualStatsChart.vue';

export default {
  components: {
    IndividualStatsChart,
  },
  props: [
    'completedLegendMarkup',
    'stats',
    'statshistory',
    'appid',
    'fns',
    'showGeneralCharts',
    'showAppCharts',
  ],
  data() {
    return {
      queuedChartConfig: chartConfig.QUEUED,
      runningChartConfig: chartConfig.RUNNING,
      completedChartConfig: chartConfig.COMPLETED,

      startingChartConfig: chartConfig.STARTING,
      waitingChartConfig: chartConfig.WAITING,
      busyChartConfig: chartConfig.BUSY,
      idlingChartConfig: chartConfig.IDLING,
      pausedChartConfig: chartConfig.PAUSED,
    }
  }
}
</script>

<style>
@media only screen and (min-width: 1500px) {
  .singleChart {
    width: 25%;
    float: left;
  }
}

@media only screen and (min-width: 1160px) and (max-width: 1499px) {
  .singleChart {
    width: 33%;
    float: left;
  }
}

@media only screen and (min-width: 450px) and (max-width: 1159px) {
  .singleChart {
    width: 50%;
    float: left;
  }
}

@media only screen and (max-width: 449) {
  .singleChart {
    width: 100%;
    float: left;
  }
}


.chart-title {
  text-align: center;
  padding-bottom: 10px;
}
</style>
