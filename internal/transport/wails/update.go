package wailstransport

import (
	"context"
	"errors"

	appupdate "github.com/openmodu/onecatch/internal/service/appupdate"
)

type UpdateBinding struct {
	serviceSource func() *appupdate.Service
}

func NewUpdateBinding(serviceSource func() *appupdate.Service) *UpdateBinding {
	return &UpdateBinding{serviceSource: serviceSource}
}

func (b *UpdateBinding) GetStatus() appupdate.Status {
	service, _ := b.current()
	if service == nil {
		return appupdate.Status{}
	}
	return service.Status()
}

func (b *UpdateBinding) Check() (appupdate.Status, error) {
	service, err := b.current()
	if err != nil {
		return appupdate.Status{}, err
	}
	return service.Check(context.Background())
}

func (b *UpdateBinding) Download() (appupdate.Status, error) {
	service, err := b.current()
	if err != nil {
		return appupdate.Status{}, err
	}
	return service.Download(context.Background())
}

func (b *UpdateBinding) Apply() error {
	service, err := b.current()
	if err != nil {
		return err
	}
	return service.Apply(context.Background())
}

func (b *UpdateBinding) current() (*appupdate.Service, error) {
	if b == nil || b.serviceSource == nil {
		return nil, errors.New("app update: service is not ready")
	}
	service := b.serviceSource()
	if service == nil {
		return nil, errors.New("app update: service is not ready")
	}
	return service, nil
}
