# Schema设计V1

## 目的
（1）解决数据库操作与业务本身逻辑实现松耦合；  
（2）在serverless场景，支持多数据库场景；  
（3）协作开发写下的安全场景；  
...  

## 关键字（不区分大小写）

### FIND 和 FIND_ONE

```
// Collection reference
col := sess.Collection("people")

// Requesting all items in the "people" collection
res := col.Find()
...

// String-like syntax is accepted.
res = col.Find("id", 25)

// Equality is the default operator but a different one can be used.
res = col.Find("id >", 29)

// The `?` placeholder maps arguments by order.
res = col.Find("id > ? AND id < ?", 20, 39)

// You can add more conditions to `Find()` with `And()`
res = col.Find("id >", 20).And("id <", 39)

// Primary keys can also be passed as arguments.
res = col.Find(20)
```

###  