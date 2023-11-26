const StatsParser = require('./stats-parser.js');

/**
 * This class is used to parse Javascript literal object data e.g.
 * {appId="01FDACB01",fnID="01FDACC02",image="fnproject/hello"}
 */
module.exports = class LabelParser extends StatsParser {
  constructor() {
    super();

    this._regex = RegExp(this._labelRE + ',?', 'gm');

    this._WHOLE_MATCH = 0;
    this._LABEL_KEY = 1;
    this._LABEL_VALUE = 2;
  }

  parse(data) {
    // Remove the start and end curly braces as they're not part of the data
    data = data.replace(/^{|}$/g, '');

    var jsonData = {};

    var labelData;
    while((labelData = this._regex.exec(data)) !== null) {
        var labelKey = labelData[this._LABEL_KEY];
        var labelValue = labelData[this._LABEL_VALUE];

        jsonData[labelKey] = labelValue;
    }

    return jsonData;
  }
};
