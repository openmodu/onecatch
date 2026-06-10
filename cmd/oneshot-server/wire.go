//go:build wireinject

package main

import (
	"github.com/google/wire"
	"github.com/openmodu/oneshot/internal/app/config"
	"github.com/openmodu/oneshot/internal/data"
	"github.com/openmodu/oneshot/internal/service"
	"github.com/openmodu/oneshot/internal/usecase"
)

func initializeServices(cfg config.Config) (*service.Services, func(), error) {
	wire.Build(
		provideMySQLDSN,
		data.ProviderSet,
		usecase.ProviderSet,
		service.ProviderSet,
	)
	return nil, nil, nil
}
