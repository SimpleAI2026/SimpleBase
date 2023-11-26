/**
 * The Stats Parser class provides base functionality that individual stats
 * parsers can reuse. It's designed for individual parsers to inherit from.
 */
module.exports = class StatsParser {
    constructor() {
      // Captures the key in a Javascript Object Literal (i.e. everything before the equals sign)
      var labelNameRE = '([^=]+)';

      // Captures the value in a Javascript Object Literal (i.e. anything between the double quotes)
      var labelValueRE = '"([^"]*)"';

      // Matches the key/value pair in a Javascript Object Literal
      this._labelRE = labelNameRE + '=' + labelValueRE;

      this._spacesRE = '\\s+';
      this._valueRE = '(\\d+)';
    }
};
