var express = require('express');
var router = express.Router();
var helpers = require('../helpers/app-helpers.js');

router.get('/', function(req, res) {
  var successcb = function(data){
    res.json(data.items);
  };
  helpers.getApiEndpoint(req, "/v2/apps", {}, successcb, helpers.standardErrorcb(res));
});

router.get('/:app', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  helpers.getApiEndpoint(req, "/v2/apps/" + encodeURIComponent(req.params.app), {}, successcb, helpers.standardErrorcb(res));
});

// Create New App
router.post('/', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };
  var data = req.body;

  helpers.postApiEndpoint(req, "/v2/apps", {}, data, successcb, helpers.standardErrorcb(res));
});

// Update App
router.put('/:app', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  var data = req.body;
  delete data.id;
  delete data.name;
  delete data.created_at;
  delete data.updated_at;

  helpers.execApiEndpoint('PUT', req,  "/v2/apps/" + encodeURIComponent(req.params.app) , {}, data, successcb, helpers.standardErrorcb(res));
});

// Delete App
router.delete('/:app', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  helpers.execApiEndpoint('DELETE', req,  "/v2/apps/" + encodeURIComponent(req.params.app) , {}, {}, successcb, helpers.standardErrorcb(res));
});


module.exports = router;
