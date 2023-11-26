const logger = require('config-logger');

const LabelParser = require('./label-parser.js');
const StatsParser = require('./stats-parser.js');

/**
 * This class is used to parse app specific stats from the Fn server's metrics
 * API.
 */
module.exports = class AppStatsParser extends StatsParser {
  constructor() {
    super();

    this._metricNames = {
      'fn_container_start_total': 'Starting',
      'fn_container_busy_total': 'Busy',
      'fn_container_idle_total': 'Idling',
      'fn_container_paused_total': 'Paused',
      'fn_container_wait_total': 'Waiting',
    };

    var metricNameRE = '(' + Object.keys(this._metricNames).join('|') + ')';

    // Match fn container info e.g. {app_id="01D8JQSKDENG8G00GZJ000000B"}
    var fnDataRE = '{' + '[^}]+' + '}';

    // unfortunately we cannot use ((?:, labelRE)+) as it won't capture the
    // middle key-value pair
    var metricsRE = '^' + metricNameRE + '(' + fnDataRE + ')' + this._spacesRE + this._valueRE;

    this._regex = RegExp(metricsRE, 'gm');

    //regexMatch[0] = the whole match
    //regexMatch[1] = metric name (e.g. fn_container_busy_total)
    //regexMatch[2] = fn data in javascript object notation (e.g. {app_id="01D7"})
    //regexMatch[3] = metric value (integer)
    this._WHOLE_MATCH = 0;
    this._METRIC_NAME = 1;
    this._FN_DATA = 2;
    this._METRIC_VALUE = 3;
  }

  /*
   * Parse the stats data and return an object containing the app specific
   * stats.
   *
   * Example data structure for object being returned:
   * 01D8JQSKDENG8G00GZJ000000B
   *  Functions
   *    01D8JQSQ2VNG8G00GZJ000000C
   *      Busy: 1
   *      Idling: 0
   *      Paused: 0
   *      Starting: 0
   *      Waiting: 0
   *    01D8JQSQ2VNG8G00GZJ000000D
   *      Busy: 0
   *      Idling: 0
   *      Paused: 0
   *      Starting: 0
   *      Waiting: 1
   *
   * @param {String} data   the data to parse.
   *
   * @return {Object}       an object representing the parsed data as per the
   *                        documentation above.
   */
  parse(data) {
    var jsonData = {};

    var labelParser = new LabelParser();
    var metricData;
    while((metricData = this._regex.exec(data)) !== null) {
      logger.debug("Processing App Stat: " + metricData[0]);

      var metricsName = metricData[this._METRIC_NAME];
      var metricsHumanName = this._metricNames[metricsName];
      var metricsValue = parseInt(metricData[this._METRIC_VALUE]);

      var rawFnData = metricData[this._FN_DATA];
      var fnData = labelParser.parse(rawFnData);

      jsonData = this._addData(jsonData, fnData.app_id, fnData.fn_id,
        metricsHumanName, metricsValue
      );
    }

    return jsonData;
  }

  /**
   * Adds App Data to the object that we're going to return.
   *
   * @param {Object} data   the object to append the data to.
   * @param {String} appId  the ID of the Fn App which this data belongs to.
   * @param {String} fnId   the ID of the Fn function which this data belongs to.
   * @param {String} metricsHumanName   the human readable name of the metric being recorded.
   * @param {Int} metricsValue  the value of the metric that was parsed.
   *
   * @return {Object}   the data object with the app data added.
   */
  _addData(data, appId, fnId, metricsHumanName, metricsValue) {
    if(data[appId] === undefined) {
      data[appId] = {'Functions': {}};
    }

    if(data[appId].Functions[fnId] === undefined) {
      data[appId].Functions[fnId] = {};
    }

    // Aggregate data for all fn images
    if(metricsHumanName in data[appId].Functions[fnId]) {
      data[appId].Functions[fnId][metricsHumanName] += metricsValue;
    } else {
      data[appId].Functions[fnId][metricsHumanName] = metricsValue;
    }

    return data;
  }
};
