package database

type DbConfig struct {
	Driver   string // driver, privide(PostgreSQL, MySQL, CockroachDB, Microsoft SQL Server, SQLite, QL and MongoDB)
	UserName string
	Password string
	Host     string
	Database string
	Options  map[string]string
}
