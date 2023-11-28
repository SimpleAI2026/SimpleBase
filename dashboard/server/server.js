const logger = require('config-logger');

const express = require('express');
const path = require('path');
const url = require('url');
const bodyParser = require('body-parser');

const app = express();

const apiUrl = url.parse(process.env.FN_API_URL);
if (!apiUrl || !apiUrl.hostname) {
  logger.error("API URL not set. Please specify Functions API URL via environment variable, e.g. FN_API_URL=http://localhost:8080 npm start");
  process.exit(1);
}

app.set('api-url', apiUrl);

const port = process.env.PORT || 4000;
const publicPath = path.resolve(__dirname, '../');

logger.info("publicPath: ", publicPath);

app.use(express.static(publicPath));
app.use(bodyParser.json());
app.use(require('./router.js'));
app.disable('etag');

app.listen(port, function () {
  logger.info('Using API url: ' + apiUrl.host);
  logger.info('Server running on port ' + port);
});
