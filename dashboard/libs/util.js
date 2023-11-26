import { eventBus } from '../main';



export const defaultErrorHandler = function (jqXHR) {
  var text = "Something went terribly wrong (Status Code: " + jqXHR.status + ")";
  try {
    text = jqXHR.responseJSON.msg;
  } catch (_err) {
    // continue regardless of error
  }
  eventBus.$emit('NotificationError', text);
};


// lines is array in format [{key: "", value: ""}]
// config is key-value hash
export const configToLines = function (config) {
  config = config || {};
  var lines = [];
  var k;
  for (k in config) {
    lines.push({ key: k, value: config[k] });
  }
  // Always show at least one empty line
  if (lines.length == 0) {
    lines.push(newConfig());
  }
  return lines;
};

export const linesToConfig = function (lines) {
  var config = {};
  for (var k = 0, i = 0, len = lines.length; i < len; k = ++i) {
    var v = lines[k];
    if (v.key) {
      config[v.key] = v.value;
    }
  }
  return config;
};

// Initialises and returns an empty config object
export const newConfig = function () {
  // 'delete' indicates when this config should be deleted from the server.
  // 'new' indicates that this config value has just been added and means the
  // key should be editable on the UI.
  return { key: "", value: "", delete: false, new: true };
};

export const headersToLines = function (headers) {
  headers = headers || {};
  var lines = [];
  var k;
  for (k in headers) {
    lines.push({ key: k, value: headers[k][0] });
  }
  // Always show at least one empty line
  if (lines.length == 0) {
    lines.push({ key: "", value: "" });
  }
  return lines;
};

export const linesToHeaders = function (lines) {
  var headers = {};
  for (var k = 0, i = 0, len = lines.length; i < len; k = ++i) {
    var v = lines[k];
    if (v.key) {
      headers[v.key] = [v.value];
    }
  }
  return headers;
};

export const getApiUrl = function (cb, errCb) {
  errCb = errCb || null;
  $.ajax({
    headers: { 'Authorization': getAuthToken() },
    url: '/api/info/api-url',
    method: 'GET',
    contentType: "application/json",
    dataType: 'json',
    success: (res) => {
      cb(res.url);
    },
    error: function (jqXHR, textStatus, errorThrown) {
      if (errCb) {
        errCb(jqXHR, textStatus, errorThrown);
      }
    }
  });
};

export const getAuthToken = function () {
  return window.localStorage['FN_TOKEN'];
};
