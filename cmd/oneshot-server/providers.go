package main

import (
	"github.com/openmodu/oneshot/internal/app/config"
	"github.com/openmodu/oneshot/internal/data"
)

func provideMySQLDSN(cfg config.Config) data.MySQLDSN {
	return data.MySQLDSN(cfg.MySQLDSN)
}
