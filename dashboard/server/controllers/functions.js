let express = require('express');
let router = express.Router();
let helpers = require('../helpers/app-helpers.js');
let logger = require('config-logger');

router.get('/', function (req, res) {
  let successcb = function (data) {
    res.json(data.items);
  };
  helpers.getApiEndpoint(req,
    "/v2/fns",
    req.query,
    successcb,
    helpers.standardErrorcb(res),
    {
      "next_cursor": "string",
      "items": [
        {
          "id": "string",
          "name": "string",
          "app_id": "string",
          "image": "string",
          "memory": 0,
          "timeout": 30,
          "idle_timeout": 30,
          "config": {
            "additionalProp1": "string",
            "additionalProp2": "string",
            "additionalProp3": "string"
          },
          "annotations": {
            "additionalProp1": {},
            "additionalProp2": {},
            "additionalProp3": {}
          },
          "created_at": "2023-11-27T13:44:49.366Z",
          "updated_at": "2023-11-27T13:44:49.366Z",
          "shape": "GENERIC_X86"
        }
      ]
    });
});

router.get('/:fn', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };
  helpers.getApiEndpoint(req,
    "/v2/fns/" + encodeURIComponent(req.params.fn),
    {},
    successcb,
    helpers.standardErrorcb(res),
    {
      "id": "string",
      "name": "string",
      "app_id": "string",
      "image": "string",
      "memory": 0,
      "timeout": 30,
      "idle_timeout": 30,
      "config": {
        "additionalProp1": "string",
        "additionalProp2": "string",
        "additionalProp3": "string"
      },
      "annotations": {
        "additionalProp1": {},
        "additionalProp2": {},
        "additionalProp3": {}
      },
      "created_at": "2023-11-27T13:45:17.998Z",
      "updated_at": "2023-11-27T13:45:17.998Z",
      "shape": "GENERIC_X86"
    });
});

// Create New Fn
router.post('/', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };
  let data = req.body;
  helpers.postApiEndpoint(req, "/v2/fns", {}, data, successcb, helpers.standardErrorcb(res));
});

// Update Fn
router.put('/:fn', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };

  let data = req.body;
  delete data.id;
  delete data.created_at;
  delete data.updated_at;

  helpers.execApiEndpoint('PUT', req, "/v2/fns/" + encodeURIComponent(req.params.fn), {}, data, successcb, helpers.standardErrorcb(res));
});

// Delete Fn
router.delete('/:fn', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };
  helpers.execApiEndpoint('DELETE', req, "/v2/fns/" + encodeURIComponent(req.params.fn), {}, {}, successcb, helpers.standardErrorcb(res));
});

// Run Function
router.post('/invoke/:fn', function (req, res) {
  let successcb = function (data) {
    res.json({ output: data });
  };

  let errcb = function (status, err) {
    logger.error("Error. Api responded with ", status, err);
    let text = "Something went terribly wrong (Status Code: " + status + ") ";
    if (err) {
      try {
        let parsed = JSON.parse(err);
        if (parsed && parsed.error && parsed.error.message) {
          text = parsed.error.message;
        }
        if (parsed.request_id) {
          text += "\n request_id: " + parsed.request_id;
        }
      } catch (e) {
        // continue regardless of error
        console.error("e: ", e);
      }
    }
    res.status(400).json({ msg: text });
  };

  // If the payload is JSON parse it, otherwise use the plain text
  let data;
  try {
    data = JSON.parse(req.body.payload);
  } catch (e) {
    data = req.body.payload;
  }

  helpers.execApiEndpointRaw('POST', req, "/invoke/" + encodeURIComponent(req.params.fn), {}, data, successcb, errcb);
});

module.exports = router;
