package sql

import (
	stdsql "database/sql"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Sql struct {
	db *gorm.DB
}

type MySQLOption func(*mysqlConfig)

type mysqlConfig struct {
	gormConfig      *gorm.Config
	maxIdleConns    int
	maxOpenConns    int
	connMaxLifetime time.Duration
}

func NewMySQL(dsn string, opts ...MySQLOption) (*Sql, error) {
	cfg := mysqlConfig{
		maxIdleConns:    10,
		maxOpenConns:    100,
		connMaxLifetime: time.Hour,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	gormOptions := make([]gorm.Option, 0, 1)
	if cfg.gormConfig != nil {
		gormOptions = append(gormOptions, cfg.gormConfig)
	}

	db, err := gorm.Open(mysql.Open(dsn), gormOptions...)
	if err != nil {
		return nil, err
	}

	raw, err := db.DB()
	if err != nil {
		return nil, err
	}
	raw.SetMaxIdleConns(cfg.maxIdleConns)
	raw.SetMaxOpenConns(cfg.maxOpenConns)
	raw.SetConnMaxLifetime(cfg.connMaxLifetime)

	return &Sql{db: db}, nil
}

func WithGormConfig(config *gorm.Config) MySQLOption {
	return func(cfg *mysqlConfig) {
		cfg.gormConfig = config
	}
}

func WithMaxIdleConns(n int) MySQLOption {
	return func(cfg *mysqlConfig) {
		cfg.maxIdleConns = n
	}
}

func WithMaxOpenConns(n int) MySQLOption {
	return func(cfg *mysqlConfig) {
		cfg.maxOpenConns = n
	}
}

func WithConnMaxLifetime(d time.Duration) MySQLOption {
	return func(cfg *mysqlConfig) {
		cfg.connMaxLifetime = d
	}
}

func (s *Sql) Gorm() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Sql) RawDB() (*stdsql.DB, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return s.db.DB()
}

func (s *Sql) Close() error {
	raw, err := s.RawDB()
	if err != nil || raw == nil {
		return err
	}
	return raw.Close()
}
