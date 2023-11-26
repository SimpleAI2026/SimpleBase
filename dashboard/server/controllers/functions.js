var express = require('express');
var router = express.Router();
var helpers = require('../helpers/app-helpers.js');
var logger = require('config-logger');

router.get('/', function(req, res) {
  var successcb = function(data){
    res.json(data.items);
  };

  helpers.getApiEndpoint(req, "/v2/fns", req.query, successcb, helpers.standardErrorcb(res));
});

router.get('/:fn', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  helpers.getApiEndpoint(req, "/v2/fns/" + encodeURIComponent(req.params.fn), {}, successcb, helpers.standardErrorcb(res));
});

// Create New Fn
router.post('/', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };
  var data = req.body;

  helpers.postApiEndpoint(req, "/v2/fns", {}, data, successcb, helpers.standardErrorcb(res));
});

// Update Fn
router.put('/:fn', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  var data = req.body;
  delete data.id;
  delete data.created_at;
  delete data.updated_at;

  helpers.execApiEndpoint('PUT', req,  "/v2/fns/" + encodeURIComponent(req.params.fn), {}, data, successcb, helpers.standardErrorcb(res));
});

// Delete Fn
router.delete('/:fn', function(req, res) {
  var successcb = function(data){
    res.json(data);
  };

  helpers.execApiEndpoint('DELETE', req,  "/v2/fns/" + encodeURIComponent(req.params.fn), {}, {}, successcb, helpers.standardErrorcb(res));
});

// Run Function
router.post('/invoke/:fn', function(req, res) {
  var successcb = function(data){
    res.json({output: data});
  };
  var errcb = function(status, err){
    logger.error("Error. Api responded with ", status, err);
    var text = "Something went terribly wrong (Status Code: " + status + ") ";
    if (err){
      try {
        var parsed = JSON.parse(err);
        if (parsed && parsed.error && parsed.error.message){
          text = parsed.error.message;
        }
        if (parsed.request_id){
          text += "\n request_id: " + parsed.request_id;
        }
      } catch (e) {
        // continue regardless of error
      }
    }
    res.status(400).json({msg: text});
  };

  // If the payload is JSON parse it, otherwise use the plain text
  var data;
  try {
    data = JSON.parse(req.body.payload);
  } catch(e) {
    data = req.body.payload;
  }

  helpers.execApiEndpointRaw('POST', req,  "/invoke/" + encodeURIComponent(req.params.fn), {}, data, successcb, errcb);
});

module.exports = router;
