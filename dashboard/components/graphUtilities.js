// Utility functions used by the graph components

// Update the graph and legend for the specified chart using the data in chart.stats and chart.statshistory
export function updateChart(chart) {

  // work out the function to extract the required metric from a stats object from JSON
  var metricGetter = chart.chartConfig.METRIC_GETTER;

  var isStacked = chart.chartConfig.isStacked;

  if (chart.statshistory && chart.stats) {
    chart.datacollection = {};
    chart.datacollection["labels"] = chart.statshistory.map(() => "");
    chart.datacollection["datasets"] = [];

    var totalCount = 0;

    if (chart.appid == null) {
      totalCount = displayGeneralMetric(chart, metricGetter, isStacked);
    } else {
      // display metrics for a specific app

      var thisApp = chart.stats.Apps[chart.appid];
      totalCount = processAppMetrics(chart, metricGetter, isStacked, thisApp);
    }

    chart.total = totalCount;
  }
}

// extract app and fn details and display metrics for them accordingly
function processAppMetrics(chart, metricGetter, isStacked, app) {
  if (app === undefined) {
    // There are no stats for this app e.g. if none of the functions
    // have been run since the server started
    return 0;
  }

  var totalCount = 0;

  // Create a cache mapping of fnId to fnName so we don't need to keep
  // iterating over the array trying to find the correct name for an id
  var fnsCache = {};
  chart.fns.forEach(function (fn) {
    fnsCache[fn.id] = fn.name;
  });

  for (var fnId in app.Functions) {

    // Handle the cases where the fn server doesn't return fnIds in its
    // metrics API or if the server doesn't know the name of the function
    var fnName = fnsCache[fnId];
    if (!fnName) {
      fnName = 'Unknown Function';
    }

    var appDetails = {
      'appId': chart.appid,
      'fnId': fnId,
      'fnName': fnName,
    };

    totalCount += displayAppMetric(chart, metricGetter, isStacked, appDetails);
  }

  return totalCount;
}

// display a single line on the specified chart, showing historical values of
// the metric in addition, return the current value of the metric
function displayGeneralMetric(chart, metricGetter, isStacked) {
  var value = getMetricFor(chart.stats, metricGetter);

  // assemble an array containing historical values of the metric that this chart is displaying
  var plotHistory = [];
  for (var i = 0; i < chart.statshistory.length; i++) {
    plotHistory.push(getMetricFor(chart.statshistory[i], metricGetter));
  }

  // The identifier is just used to map what colour this series should be
  // listed as. As general metrics only have one series we don't need to
  // use a unique identifier
  var identifier = 'generalMetric';

  var dataSet = generateDataSet(
    'Amount',
    isStacked,
    getBackgroundColorFor(identifier),
    getBorderColorFor(identifier),
    plotHistory
  );

  chart.datacollection["datasets"].push(dataSet);

  return value;
}

// display a single line on a chart detailing app metrics. In addition, return
// the current value of the metric
function displayAppMetric(chart, metricGetter, isStacked, appDetails) {
  var appStats = getFnStatsFromApp(chart.stats, appDetails);
  var value = getMetricFor(appStats, metricGetter);

  var plotHistory = [];
  for (var i = 0; i < chart.statshistory.length; i++) {
    var historicStat = getFnStatsFromApp(chart.statshistory[i], appDetails);
    var statsHistory = getMetricFor(historicStat, metricGetter);

    plotHistory.push(statsHistory);
  }

  // Create a unique identifier for this metric
  var identifier = Object.values(appDetails).join('-');

  var dataSet = generateDataSet(
    appDetails.fnName,
    isStacked,
    getBackgroundColorFor(identifier),
    getBorderColorFor(identifier),
    plotHistory
  );

  chart.datacollection["datasets"].push(dataSet);

  return value;
}

// get stats object for Fn using the passed in appDetails
function getFnStatsFromApp(stats, appDetails) {
  try {
    return stats.Apps[appDetails.appId].Functions[appDetails.fnId];
  } catch (e) {
    // The keys in the stats history won't exist to start with so just return
    // null rather than crashing in this case
    if (e instanceof TypeError) {
      return null;
    } else {
      throw e;
    }
  }
}

// generate the Dataset that the Vue chart will use
function generateDataSet(valueName, isStacked, backgroundColor, borderColor, statHistory) {
  return {
    label: valueName,
    // Use a fill color to distingusih stacked and non-stacked charts
    fill: isStacked,
    // Use a fill color for stacked charts
    backgroundColor: isStacked ? backgroundColor : 'white',
    borderColor: borderColor,
    borderWidth: lineWidthInPixels,
    radius: pointRadiusInPixels,
    data: statHistory
  };
}

// return the metric value from the specified stats object. If the stats
// object doesn't exist then zero is returned
function getMetricFor(stats, metricGetter) {
  if (stats == null) {
    // we didn't have any information about this app at the time this historical stat was added
    // either we have a partially-initialised statshistory or the app has not been created yet
    return 0;
  } else {
    return metricGetter(stats);
  }
}

export var chartConfig = {

  // General charts
  QUEUED: {
    NAME: 'Queued',
    LEGEND_DIV_NAME: 'queuedChartLegend',
    METRIC_GETTER: results => results.Queue || 0,
    IS_STACKED: false,
    SHOW_LEGEND: false,
  },
  RUNNING: {
    NAME: 'Running',
    LEGEND_DIV_NAME: 'runningChartLegend',
    METRIC_GETTER: results => results.Running || 0,
    IS_STACKED: false,
    SHOW_LEGEND: false,
  },
  COMPLETED: {
    NAME: 'Completed',
    LEGEND_DIV_NAME: 'completedChartLegend',
    METRIC_GETTER: results => results.Complete || 0,
    IS_STACKED: true,
    SHOW_LEGEND: false,
  },

  // Charts for app data
  BUSY: {
    NAME: 'Busy',
    LEGEND_DIV_NAME: 'busyChartLegend',
    METRIC_GETTER: results => results.Busy || 0,
    IS_STACKED: false,
    SHOW_LEGEND: true,
  },
  IDLING: {
    NAME: 'Idling',
    LEGEND_DIV_NAME: 'idlingChartLegend',
    METRIC_GETTER: results => results.Idling || 0,
    IS_STACKED: false,
    SHOW_LEGEND: true,
  },
  PAUSED: {
    NAME: 'Paused',
    LEGEND_DIV_NAME: 'pausedChartLegend',
    METRIC_GETTER: results => results.Paused || 0,
    IS_STACKED: false,
    SHOW_LEGEND: true,
  },
  STARTING: {
    NAME: 'Starting',
    LEGEND_DIV_NAME: 'startingChartLegend',
    METRIC_GETTER: results => results.Starting || 0,
    IS_STACKED: false,
    SHOW_LEGEND: true,
  },
  WAITING: {
    NAME: 'Waiting',
    LEGEND_DIV_NAME: 'waitingChartLegend',
    METRIC_GETTER: results => results.Waiting || 0,
    IS_STACKED: false,
    SHOW_LEGEND: true,
  },
};

// factory for background colors; simply iterate round these arrays of colors
const backgroundColors = ['rgba(255, 99, 132, 0.2)', 'rgba(54, 162, 235, 0.2)', 'rgba(255, 206, 86, 0.2)', 'rgba(75, 192, 192, 0.2)', 'rgba(153, 102, 255, 0.2)', 'rgba(255, 159, 64, 0.2)'];
const borderColors = ['rgba(255,99,132,1)', 'rgba(54, 162, 235, 1)', 'rgba(255, 206, 86, 1)', 'rgba(75, 192, 192, 1)', 'rgba(153, 102, 255, 1)', 'rgba(255, 159, 64, 1)'];

export const lineWidthInPixels = 1;
export const pointRadiusInPixels = 0.5;

var backgroundColorMap = {};
export function getBackgroundColorFor(path) {
  if (!backgroundColorMap[path]) {
    backgroundColorMap[path] = backgroundColors[(Object.keys(backgroundColorMap).length) % (backgroundColors.length)];
  }
  return backgroundColorMap[path];
}

var borderColorMap = {};
export function getBorderColorFor(path) {
  if (!borderColorMap[path]) {
    borderColorMap[path] = borderColors[(Object.keys(borderColorMap).length) % (borderColors.length)];
  }
  return borderColorMap[path];
}

