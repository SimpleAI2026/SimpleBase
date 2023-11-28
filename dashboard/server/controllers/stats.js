let express = require('express');
let router = express.Router();

let AppStatsParser = require('../helpers/app-stats-parser.js');
let GeneralStatsParser = require('../helpers/general-stats-parser.js');
let helpers = require('../helpers/app-helpers.js');

router.get('/', function (req, res) {
  let successcb = function (data) {
    // convert the raw Prometheus data to JSON in a form usable by the client
    let generalStatsParser = new GeneralStatsParser();
    let appStatsParser = new AppStatsParser();
    let jsonData = generalStatsParser.parse(data);
    let appData = appStatsParser.parse(data);
    jsonData['Apps'] = appData;
    res.json(jsonData);
  };

  // get the raw Prometheus data
  helpers.getApiEndpointRaw(req, "/metrics", {}, successcb, helpers.standardErrorcb(res), {});
});

module.exports = router;
