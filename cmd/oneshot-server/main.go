package main

import (
	"context"
	"os"

	"github.com/openmodu/oneshot/internal/app/config"
	"github.com/openmodu/oneshot/internal/app/logger"
	httptransport "github.com/openmodu/oneshot/internal/transport/http"
	"github.com/openmodu/oneshot/pkg/server"
)

func main() {
	log := logger.New()
	cfg := config.Load()

	services, cleanup, err := initializeServices(cfg)
	if err != nil {
		log.Error("initialize services failed", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := server.Run(context.Background(), log, server.Config{
		Addr: cfg.HTTPAddr,
	}, httptransport.NewServer(services)); err != nil {
		log.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}
