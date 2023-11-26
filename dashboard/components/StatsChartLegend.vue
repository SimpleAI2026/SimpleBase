<template>
  <div :id="chartConfig.LEGEND_DIV_NAME" v-if="chartConfig.SHOW_LEGEND">
    <ul class="chartLegend">
      <li v-for="chartDataSet in datacollection['datasets']">
        <span class="chartLabelEmblem" :style="{
          backgroundColor: chartDataSet.backgroundColor,
          borderColor: chartDataSet.borderColor
        }"></span>
        <span class="chartLabelText">
          {{ chartDataSet.label }}: {{ chartDataSet.data.slice(-1).toString() }}
        </span>
      </li>
    </ul>
  </div>
</template>

<script>
export default {
  props: [
    'chartConfig',
    'datacollection'
  ],
}
</script>

<style>
.chartLabelEmblem {
  float: left;
  width: 40px;
  height: 14px;
  line-height: 20px;
  margin-top: 3px;
  margin-left: 10px;
  margin-right: 6px;
  border: 1px solid;
  font-size: 8px;
}

.chartLabelText {
  font-size: 14px;
}

ul.chartLegend {
  margin-right: 10px;
  list-style-type: none;
  padding-left: 0px;
}
</style>
