# users
<!-- allow: user -->

`user list|get|create|update|disable|delete`。密码只从 stdin 读取。`user delete` 与 `user disable` 需要确认。没有 `user:admin` 时写操作返回 `role_forbidden`，不要重试。
