const url = require('url');
const request = require('request');
const logger = require('config-logger');

const apiFullUrl = function (req, path) {
  const apiUrl = req.app.get('api-url');
  const httpurl = url.format(apiUrl) + path.replace(/^\//, "");
  return httpurl;
};

const addAuth = function (options, req) {
  if (req.get('Authorization') !== undefined) {
    options.headers = {
      'Authorization': req.get('Authorization')
    };
  }
  return options;
};

const isMockData = true; // TODO: mock数据

// expects response as json
const requestCB = function (successcb, errorcb, error, response, body) {
  let parsed;

  if (!error && response.statusCode >= 200 && response.statusCode < 300) {
    try {
      if (typeof body == "string") {
        parsed = JSON.parse(body);
      } else {
        parsed = body;
      }
    } catch (e) {
      logger.error("Can not parse json: ", body, e);
    }

    // A 204 status code indicates a success but there won't be any data. E.g.
    // on the deletion of an app. This will throw an error down the line so
    // initialise parsed.
    if (!parsed && response.statusCode == 204) {
      parsed = {};
    }

    if (parsed) {
      successcb(parsed);
    } else {
      errorcb(response.statusCode, "Can not parse api response");
    }
  } else {
    let message;
    try {
      if (typeof body == "string") {
        parsed = JSON.parse(body);
      } else {
        parsed = body;
      }
      if (parsed) {
        if (parsed.error && parsed.error.message) {
          message = parsed.error.message;
        } else if (parsed.message) {
          message = parsed.message;
        }
      }
    } catch (e) {
      message = "Can not parse api response";
    }
    message = message || "An error ocurred.";
    const status = response ? response.statusCode : error.code;
    logger.error("[ERR] " + status + " | " + message);
    errorcb(status, message);
  }
};

// expects response as plain text
const requestCBRaw = function (successcb, errorcb, error, response, body) {
  if (!error && response.statusCode >= 200 && response.statusCode < 300) {
    successcb(body);
  } else {
    const status = response ? response.statusCode : error.code;
    errorcb(status, body);
  }
};

exports.extend = function (target) {
  const sources = [].slice.call(arguments, 1);
  sources.forEach(function (source) {
    for (const prop in source) {
      target[prop] = source[prop];
    }
  });
  return target;
};

exports.getApiEndpoint = function (req, path, params, successcb, errorcb, mockData) {
  if (mockData && isMockData) {
    logger.info("isMockData: ", mockData);
    successcb(mockData);
    return;
  }

  const url = apiFullUrl(req, path);
  logger.debug("GET " + url + ", params: ", params);
  let options = { url: url, qs: params };
  options = addAuth(options, req);

  request(
    options,
    function (error, response, body) {
      requestCB(successcb, errorcb, error, response, body);
    }
  );
};

exports.getApiEndpointRaw = function (req, path, params, successcb, errorcb, mockData) {
  if (mockData && isMockData) {
    logger.info("isMockData: ", mockData);
    successcb(mockData);
    return;
  }

  const url = apiFullUrl(req, path);
  logger.debug("GET " + url + ", params: ", params);
  let options = { url: url, qs: params };
  options = addAuth(options, req);

  request(
    options,
    function (error, response, body) {
      requestCBRaw(successcb, errorcb, error, response, body);
    }
  );
};

exports.postApiEndpoint = function (req, path, params, postfields, successcb, errorcb) {
  execApiEndpoint('POST', req, path, params, postfields, successcb, errorcb);
};

exports.execApiEndpoint = function (method, req, path, params, postfields, successcb, errorcb) {
  let options = {
    uri: apiFullUrl(req, path),
    method: method,
    json: postfields
  };
  logger.debug(options.method + " " + options.uri + ", params: ", options.body);
  options = addAuth(options, req);

  request(
    options,
    function (error, response, body) {
      requestCB(successcb, errorcb, error, response, body);
    }
  );
};

exports.execApiEndpointRaw = function (method, req, path, params, postfields, successcb, errorcb) {
  let options = {
    uri: apiFullUrl(req, path),
    method: method,
    json: postfields
  };

  logger.debug(options.method + " " + options.uri + ", params: ", options.body);
  options = addAuth(options, req);

  request(
    options,
    function (error, response, body) {
      requestCBRaw(successcb, errorcb, error, response, body);
    }
  );
};

exports.standardErrorcb = function (res) {
  return function (status, err) {
    logger.error("Error. Api responded with ", status, err);
    let text = "Something went terribly wrong (Status Code: " + status + ") ";
    if (err) {
      text = "Error: " + err;
    }
    res.status(400).json({ msg: text });
  };
};
