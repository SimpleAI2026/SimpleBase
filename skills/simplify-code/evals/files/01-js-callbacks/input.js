// report.js — public API: loadReport(db, id, cb) ; cb(err, report)
function loadReport(db, id, cb) {
  db.getUser(id, function (err, user) {
    if (err) {
      cb(err);
    } else {
      db.log('user ' + user.name);
      db.getOrders(user.id, function (err, orders) {
        if (err) {
          cb(err);
        } else {
          db.log('orders ' + orders.length);
          db.getInvoice(user.id, function (err, invoice) {
            if (err) {
              cb(err);
            } else {
              db.log('invoice ' + invoice.total);
              var total = 0;
              for (var i = 0; i < orders.length; i++) {
                total = total + orders[i].amount;
              }
              cb(null, { name: user.name, orders: orders.length, total: total, invoiceTotal: invoice.total });
            }
          });
        }
      });
    }
  });
}
module.exports = { loadReport };
