package config

import (
	"os"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

type Config struct {
	HTTPAddr string
	MySQLDSN string
}

func Load() Config {
	addr := os.Getenv("ONESHOT_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	return Config{
		HTTPAddr: addr,
		MySQLDSN: loadMySQLDSN(),
	}
}

func loadMySQLDSN() string {
	if dsn := os.Getenv("ONESHOT_MYSQL_DSN"); dsn != "" {
		return dsn
	}

	addr := os.Getenv("ONESHOT_MYSQL_ADDR")
	user := os.Getenv("ONESHOT_MYSQL_USER")
	password := os.Getenv("ONESHOT_MYSQL_PASSWORD")
	database := os.Getenv("ONESHOT_MYSQL_DATABASE")
	if addr == "" && user == "" && password == "" && database == "" {
		return ""
	}
	if addr == "" {
		addr = "127.0.0.1:3306"
	}
	if user == "" {
		user = "root"
	}

	cfg := drivermysql.Config{
		User:                 user,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 addr,
		DBName:               database,
		AllowNativePasswords: true,
		ParseTime:            true,
		Loc:                  time.Local,
		Params: map[string]string{
			"charset": "utf8mb4",
		},
	}
	return cfg.FormatDSN()
}
