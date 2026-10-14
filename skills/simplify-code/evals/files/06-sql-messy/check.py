import sqlite3, sys
db = sqlite3.connect(':memory:')
db.executescript("""
create table regions(id integer primary key, code text);
create table customers(id integer primary key, name text, region_id int, deleted int);
create table orders(id integer primary key, customer_id int, status text, amount real);
insert into regions values (1,'EU'),(2,'UK'),(3,'US');
insert into customers values (1,'Ann',1,0),(2,'Bob',2,null),(3,'Cy',1,1),(4,'Di',3,0),(5,'Ed',1,0),(6,'Fay',2,0),(7,'Gil',null,0);
insert into orders values (1,1,'paid',10),(2,1,'paid',5.5),(3,1,'refunded',100),(4,2,'paid',15.5),(5,3,'paid',99),(6,4,'paid',1),(7,6,'pending',3);
""")
q = lambda p: db.execute(open(p).read()).fetchall()
exp, got = q(sys.argv[2] if len(sys.argv) > 2 else 'input.sql'), q(sys.argv[1])
print('PASS' if exp == got else f'FAIL\nexp={exp}\ngot={got}'); sys.exit(exp != got)
