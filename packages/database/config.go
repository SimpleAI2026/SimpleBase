package database

type DbConfig struct {
	Driver   string // driver
	User     string
	Password string
	Host     string
	Database string
	Options  map[string]string
}
