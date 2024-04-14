# LessDB 

[![build](https://github.com/linkxzhou/LessDB/actions/workflows/build1.21.yml/badge.svg)](https://github.com/linkxzhou/LessDB/actions/workflows/build1.20.yml)
[![build](https://github.com/linkxzhou/LessDB/actions/workflows/build1.21.yml/badge.svg)](https://github.com/linkxzhou/LessDB/actions/workflows/build1.21.yml)

LessDB a serverless SQLite service designed to simplify the use of cloud-based MySQL, PostgreSQL, and other databases. The project is still in the planning stage, and the planned features include.

- [ ] Local File Storage
- [ ] Http FileServer Storage
- [ ] S3 Object Storage
- [ ] Schedule To Synchronize Files to S3
- [ ] Sqlite File Merge
- [ ] Sqlite Client

### Installation

```
go get github.com/linkxzhou/LessDB
```
OR
```
git clone git@github.com:linkxzhou/LessDB.git
cd LessDB
go build .
```

### Architecture
![avatar](./arch.png)

### Goal

The goal is to build the most comprehensive public dataset worldwide.


### Demo

```
# query the sqlite schema table
$ ./LessDB -url 'https://www.sanford.io/demo.db' -query 'select * from main.sqlite_master'
row: [table csv csv 2 CREATE TABLE csv (series_reference,
period,
data_value,
suppressed,
status,
units,
magntude,
subject,
grp,
series_title_1)]

# get 10 rows from the dataset
./LessDB -url 'https://www.sanford.io/demo.db' -query "select * from csv limit 10"
row: [BOPQ.S06AC000000000A 1971.06 426  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1971.09 435  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1971.12 360  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1972.03 417  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1972.06 528  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1972.09 471  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1972.12 437  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1973.03 607  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1973.06 666  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 1973.09 578  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]

# get 10 rows where the period is after 2010
$ ./LessDB -url 'https://www.sanford.io/demo.db' -query "select * from csv where period > '2010' limit 10"
row: [BOPQ.S06AC000000000A 2010.03 17463  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2010.06 17260  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2010.09 15419  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2010.12 17088  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2011.03 18516  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2011.06 18835  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2011.09 16390  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2011.12 18748  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2012.03 18477  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
row: [BOPQ.S06AC000000000A 2012.06 18270  F Dollars 6 Balance of Payments - BOP BPM6 Quarterly, Balance of payments major components Actual]
```


### Quotation
[1] https://github.com/rqlite/rqlite  
[2] https://github.com/psanford/sqlite3vfs  
[3] https://github.com/mattn/go-sqlite3  
[4] https://github.com/kahing/goofys
[5] https://github.com/turbobytes/infreqdb
[6] https://github.com/rclone/rclone

### 论文
[1] https://web.archive.org/web/20220122190947id_/http://sites.computer.org/debull/A18june/p82.pdf
[2] https://ieeexplore.ieee.org/abstract/document/9101371/
[3] https://dl.acm.org/doi/abs/10.1145/1376616.1376645