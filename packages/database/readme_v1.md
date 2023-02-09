# Schema设计V1

## 目的
（1）解决数据库操作与业务本身逻辑实现松耦合；  
（2）在serverless场景，支持多数据库场景；  
（3）协作开发写下的安全场景；  
...  

## 关键字（不区分大小写）
DRIVER，INIT，SQL，JSON，SELECT，UPDATE，DELETE，->，{，}等

### DRIVER

支持多数据库PostgreSQL, MySQL, CockroachDB, Microsoft SQL Server, SQLite and MongoDB等，也可以扩展原始的NoSQL执行语句；

```
DRIVER {
    Name = mysql // 驱动名称
    Databases = XXX // DB
    Uri = XXX // 操作URI
    User = root // 用户名
    Password = root // 密码
    Timeout = 3000ms // 超时设置
    ... // 等其他参数

    InitVersion = [] // 初始化需要执行的版本，与INIT关键字混合使用
}
```

### INIT

初始化执行，主要用于数据库初始化等执行，可支持两种形式：  
1、INIT中使用原始SQL
- [VERSION] SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，可支持多条语句，其中[VERSION]初始化执行的版本（默认不填，为latest版本）

2、INIT中使用Schema特性
- 支持其他非原始语句，如定于全局变量

```
INIT {
    [VERSION] SQL {
        Create table xxx1(
            ....
        );
        Create table xxx2(
        );
        Alter table XXXX1;
        Alter table XXXX2;
    } -> JSON // 执行原始SQL

    $Name = "XXX" // 定义全局变量 
}
```

### SELECT

查询语句，主要用于数据库查询操作，可支持两种形式：  
1、SELECT中使用原始SQL
- [VERSION] SelectName1 SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，支持一条语句，其中[VERSION]为查询语句运行的版本，和业务本身传入的版本决定（默认不填，为latest版本）
- SelectName1 为当前文件下面执行语句的唯一名字

```
SelectName1 {
    [VERSION] SQL {

    }
} 
```

2、SELECT中使用Schema特性
- 支持其他非原始语句，如select {} from XXX
- 支持各个数据库通用联表查询方式，如JOIN，FullJoin

```
SelectName1 { 
    [VERSION] SELECT {
        id
        name
    } From XXX
}

SelectName2 {
    [VERSION] SELECT {
        id
        name
    } 
    From XXX 
    JOIN XXX1
    Using id
}

SelectName3 { 
    [VERSION] SELECT {
        id
        name
    } 
    From XXX 
    FullJoin XXX1
    Using id
}
```

### Insert

插入语句，主要用于数据库插入操作，可支持两种形式：  
1、Insert中使用原始SQL
- InsertName1 [VERSION] SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，支持多条语句（多条语句为事务操作），其中[VERSION]为查询语句运行的版本，和业务本身传入的版本决定（默认不填，为latest版本）
- InsertName1 为当前文件下面执行语句的唯一名字

```
InsertName1 {
    [VERSION] SQL {

    }
}
```

2、Insert中使用Schema特性
- 支持其他非原始语句，如InsertInto

```
InsertName1 {
    [VERSION] InsertInto XXX {
        people = ?
        name = ?
        John = ?
    }
}
```

### UPDATE

更新语句，主要用于数据库更新操作，可支持两种形式：  
1、UPDATE中使用原始SQL
- UpdateName1 [VERSION] SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，支持多条语句（多条语句为事务操作），其中[VERSION]为查询语句运行的版本，和业务本身传入的版本决定（默认不填，为latest版本）

```
UpdateName1 {
    [VERSION] SQL {

    }
}
```

2、UPDATE 中使用Schema特性
- 支持其他非原始语句，如UPDATE XXX Set { }

```
UpdateName1 {
    [VERSION] UPDATE XXX Set {
        people = ?
        name = ?
        John = ?
    } Where {
        people = ?
    }
} 

UpdateName2 {
    [VERSION] UPDATE XXX Set {
        people = ?
        name = ?
        John = ?
    } Where {
        people = ?
        OR
        people = ?
    }
}
```

### DELETE

删除语句，主要用于数据库删除操作，可支持两种形式：  
1、DELETE中使用原始SQL
- DeleteName1 [VERSION] SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，支持多条语句（多条语句为事务操作），其中[VERSION]为查询语句运行的版本，和业务本身传入的版本决定（默认不填，为latest版本）

```
DeleteName1 {
    [VERSION] SQL {

    }
} 
```

2、DELETE中使用Schema特性
- 支持其他非原始语句，如DELETEFrom XXX Set { }

```
DeleteName1 {
    [VERSION] DELETEFrom XXXX Where {
        people = ?
    }
}
```

### Exec

执行语句，主要用于上述不能满足的一些场景的操作，可支持两种形式：  
1、Exec中使用原始SQL
- ExecName1 [VERSION] SQL { } -> JSON 大括号内填写原始SQL语句，以`;`结尾，以JSON Object返回，支持多条语句（多条语句为事务操作），其中[VERSION]为查询语句运行的版本，和业务本身传入的版本决定（默认不填，为latest版本）

```
ExecName1 {
    [VERSION] SQL {

    }
} 
```

2、Exec中不使用Schema特性

### 注释

使用 `//` 或者 `/*** ***/`

## 调用样例(伪代码)

```
...
// 初始化sqlschema文件
func init() {
    sqlschemaName1 = xxx("./name1.sqlschema")
}

// 读取指定的SelectName1的数据
func ReadDbData() {
    ...
    result := sqlschemaName1.ExecSchema("SelectName1", ...args)
    ...
}
```