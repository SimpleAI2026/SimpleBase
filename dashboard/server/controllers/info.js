let express = require('express');
let url = require('url');
let router = express.Router();

router.get('/api-url', function (req, res) {
  let apiUrl = req.app.get('api-url');
  res.json({ url: url.format(apiUrl) });
});

module.exports = router;
