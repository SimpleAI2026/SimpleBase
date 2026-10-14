select c.name, (select count(*) from orders o where o.customer_id = c.id and o.status = 'paid') as paid_orders, (select sum(o.amount) from orders o where o.customer_id = c.id and o.status = 'paid') as paid_total
from customers c, regions r
where c.region_id = r.id and r.code in ('EU','UK') and (c.deleted = 0 or c.deleted is null)
order by 3 desc, 1
