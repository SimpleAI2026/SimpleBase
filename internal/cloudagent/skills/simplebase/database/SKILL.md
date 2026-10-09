# database
<!-- allow: database,sql,schema,collection,document -->

命令：`database list|get|create|delete`，`sql query|exec|batch`，`schema show|rows|create-table|add-column`，`collection list|create`，`document list|insert|update|delete`。

需要确认：`database delete`、`document delete`，以及 SQL 首关键字为 DROP、DELETE、TRUNCATE，或 ALTER 且含 DROP。INSERT/UPDATE/CREATE 直接执行。

系统库删除会被拒绝（`system_database_protected`）。多语句返回 `multiple_statements`。
