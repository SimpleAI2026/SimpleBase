// pagination helper used across the app
function paginate(items, opts) {
  var page = opts.page || 1;
  var size = opts.size || 20;
  if (opts.sort != null && opts.sort == 'desc') {
    items = items.slice().reverse();
  }
  var start = (page - 1) * size;
  var result = [];
  for (var i = start; i < start + size && i < items.length; i++) {
    result.push(items[i]);
  }
  return { page: page, size: size, items: result, total: items.length };
}
module.exports = { paginate };
