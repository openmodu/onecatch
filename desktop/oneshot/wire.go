//go:build wireinject

package main

import (
	"github.com/google/wire"
	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

func initializeClient() oneshot.Client {
	wire.Build(
		apiBaseURL,
		oneshot.NewHTTPClient,
		wire.Bind(new(oneshot.Client), new(*oneshot.HTTPClient)),
	)
	return nil
}
