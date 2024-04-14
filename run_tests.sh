#!/bin/sh

ls -al *
echo "query select * from main.sqlite_master"
./sqlite_serverless -url 'https://www.sanford.io/demo.db' -query 'select * from main.sqlite_master'

echo "query select * from csv limit 10"
./sqlite_serverless -url 'https://www.sanford.io/demo.db' -query "select * from csv limit 10"

echo "select * from csv where period > '2010' limit 10"
./sqlite_serverless -url 'https://www.sanford.io/demo.db' -query "select * from csv where period > '2010' limit 10"