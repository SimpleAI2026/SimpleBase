const logger = require('config-logger');

const StatsParser = require('./stats-parser.js');

/**
 * This parser is used to parse the fn_queue, fn_running and fn_complete data
 * from the Fn server's metrics API. This data isn't app specific, but is for
 * the whole system.
 */
module.exports = class GeneralStatsParser extends StatsParser {
  constructor() {
    super();

    this._metricNames = {
      'fn_queued': 'Queue',
      'fn_running': 'Running',
      'fn_completed': 'Complete',
    };

    var metricNameRE = '(' + Object.keys(this._metricNames).join('|') + ')';
    var metricsRE = '^' + metricNameRE + this._spacesRE + this._valueRE;

    this._regex = RegExp(metricsRE, 'gm');

    //regexMatch[0] = the whole match
    //regexMatch[1] = metric name (e.g. fn_completed)
    //regexMatch[2] = metric value (integer)
    this._WHOLE_MATCH = 0;
    this._METRIC_NAME = 1;
    this._METRIC_VALUE = 2;
  }

  /*
   * Parse the Fn server stats data and return an object containing the
   * results.
   *
   * Example data structure for object being returned:
   * Complete: 3
   * Queue: 0
   * Running: 1
   *
   * @param {String} data   the data to parse.
   *
   * @return {Object}       an object representing the parsed data as per the
   *                        documentation above.
   */
  parse(data) {
    var jsonData = {};

    var metricData;
    while((metricData = this._regex.exec(data)) !== null) {
        logger.debug(
          "Processing General Stat: " + metricData[this._WHOLE_MATCH]
        );

        var metricsName = metricData[this._METRIC_NAME];
        var metricsHumanName = this._metricNames[metricsName];
        var metricsValue = parseInt(metricData[this._METRIC_VALUE]);

        jsonData[metricsHumanName] = metricsValue;
    }

    return jsonData;
  }
};
