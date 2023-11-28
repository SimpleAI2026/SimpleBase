let express = require('express');
let router = express.Router();
let helpers = require('../helpers/app-helpers.js');

router.get('/', function (req, res) {
  let successcb = function (data) {
    res.json(data.items);
  };

  helpers.getApiEndpoint(req,
    "/v2/apps",
    {},
    successcb,
    helpers.standardErrorcb(res),
    {
      "next_cursor": "string",
      "items": [
        {
          "id": "string",
          "name": "string",
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
          "syslog_url": "string",
          "created_at": "2023-11-27T13:42:50.487Z",
          "updated_at": "2023-11-27T13:42:50.487Z",
          "shape": "GENERIC_X86"
        }
      ]
    });
});

router.get('/:app', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };

  helpers.getApiEndpoint(req,
    "/v2/apps/" + encodeURIComponent(req.params.app),
    {},
    successcb,
    helpers.standardErrorcb(res),
    {
      "id": "string",
      "name": "string",
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
      "syslog_url": "string",
      "created_at": "2023-11-27T13:43:56.640Z",
      "updated_at": "2023-11-27T13:43:56.640Z",
      "shape": "GENERIC_X86"
    });
});

// Create New App
router.post('/', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };

  let data = req.body;
  helpers.postApiEndpoint(req, "/v2/apps", {}, data, successcb, helpers.standardErrorcb(res));
});

// Update App
router.put('/:app', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };

  let data = req.body;
  delete data.id;
  delete data.name;
  delete data.created_at;
  delete data.updated_at;

  helpers.execApiEndpoint('PUT', req, "/v2/apps/" + encodeURIComponent(req.params.app), {}, data, successcb, helpers.standardErrorcb(res));
});

// Delete App
router.delete('/:app', function (req, res) {
  let successcb = function (data) {
    res.json(data);
  };

  helpers.execApiEndpoint('DELETE', req, "/v2/apps/" + encodeURIComponent(req.params.app), {}, {}, successcb, helpers.standardErrorcb(res));
});


module.exports = router;
