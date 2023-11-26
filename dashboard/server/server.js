var logger = require('config-logger');

var express = require('express');
var path = require('path');
var url = require('url');
var bodyParser = require('body-parser');

var app = express();

var apiUrl = url.parse(process.env.FN_API_URL);
if (!apiUrl || !apiUrl.hostname) {
  logger.error("API URL not set. Please specify Functions API URL via environment variable, e.g. FN_API_URL=http://localhost:8080 npm start");
  process.exit(1);
}

app.set('api-url', apiUrl);
var port = process.env.PORT || 4000;
var publicPath = path.resolve(__dirname, '.');

app.use(express.static(publicPath));
app.use(bodyParser.json());

app.use(require('./router.js'));

app.disable('etag');

app.listen(port, function () {
  logger.info('Using API url: ' + apiUrl.host);
  logger.info('Server running on port ' + port);
});
