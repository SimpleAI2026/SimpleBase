# SimpleBase
<!-- allow: -->

通过工具 `simplebase` 调用 CLI。参数是 argv 数组，不要拼成 shell 字符串。

成功时 stdout 为 `{"ok":true,"data":...}`。失败时看 `error.code`，不要编造结果，不要输出密钥或 token。

没有被点名的子 skill 不要调用。请用户用 `@数据库`、`@键值`、`@对象存储`、`@云函数`、`@定时任务`、`@沙盒`、`@日志`、`@用户`、`@项目` 点名。

别名：数据库/database/db、键值/kv、对象存储/s3/对象、云函数/函数/gofunction、定时任务/定时/cron、沙盒/sandbox、日志/logs、用户/users、项目/project。
