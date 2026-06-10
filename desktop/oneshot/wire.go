//go:build wireinject

package main

import (
	"github.com/google/wire"
	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

func initializeClient() oneshot.Client {
	wire.Build(
		newDesktopClient,
	)
	return nil
}
