package server

import (
	"github.com/google/wire"
	serverservice "github.com/openmodu/onecatch/internal/service/server"
)

var ServiceProviderSet = wire.NewSet(serverservice.NewServices)
