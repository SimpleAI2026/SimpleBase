# LessDB 

[![build](https://github.com/linkxzhou/LessDB/actions/workflows/build1.20.yml/badge.svg)](https://github.com/linkxzhou/LessDB/actions/workflows/build1.20.yml)
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
cd LessDB/cmd/http
go build .
```

### Architecture
![avatar](./arch.png)

### Goal

The goal is to build the most comprehensive public dataset worldwide.

### Quotation
[1] https://github.com/rqlite/rqlite   
[2] https://github.com/psanford/sqlite3vfs    
[3] https://github.com/mattn/go-sqlite3    
[4] https://github.com/kahing/goofys  
[5] https://github.com/turbobytes/infreqdb  
[6] https://github.com/rclone/rclone   
[7] https://ieeexplore.ieee.org/abstract/document/9101371  
[8] https://dl.acm.org/doi/abs/10.1145/1376616.1376645  