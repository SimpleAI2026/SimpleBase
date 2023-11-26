var express = require('express');
var router = express.Router();

var AppStatsParser = require('../helpers/app-stats-parser.js');
var GeneralStatsParser = require('../helpers/general-stats-parser.js');
var helpers = require('../helpers/app-helpers.js');

router.get('/', function(req, res) {
  var successcb = function(data){
    // convert the raw Prometheus data to JSON in a form usable by the client

    var generalStatsParser = new GeneralStatsParser();
    var appStatsParser = new AppStatsParser();

    var jsonData = generalStatsParser.parse(data);
    var appData = appStatsParser.parse(data);

    jsonData['Apps'] = appData;

    res.json(jsonData);
  };

  // get the raw Prometheus data
  helpers.getApiEndpointRaw(req, "/metrics", {}, successcb, helpers.standardErrorcb(res));

});

module.exports = router;
