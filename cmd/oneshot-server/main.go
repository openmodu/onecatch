package main

import (
	"context"
	"os"

	"github.com/openmodu/oneshot/internal/app/config"
	httptransport "github.com/openmodu/oneshot/internal/transport/http"
	"github.com/openmodu/oneshot/pkg/logger"
	"github.com/openmodu/oneshot/pkg/server"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer logger.Sync(log)

	services, cleanup, err := initializeServices(cfg)
	if err != nil {
		log.Error("initialize services failed", zap.Error(err))
		os.Exit(1)
	}
	defer cleanup()

	if err := server.Run(context.Background(), log, server.Config{
		Addr: cfg.HTTPAddr,
	}, httptransport.NewServer(services)); err != nil {
		log.Error("server shutdown failed", zap.Error(err))
		os.Exit(1)
	}
}
