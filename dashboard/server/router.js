const express = require('express');
const router = express.Router();

router.use('/api/apps', require('./controllers/apps.js'));
router.use('/api/fns', require('./controllers/functions.js'));
router.use('/api/info', require('./controllers/info.js'));
router.use('/api/stats', require('./controllers/stats.js'));

module.exports = router;
