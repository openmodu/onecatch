//go:build !darwin || !cgo

package landiscovery

import "context"

func Advertise(fingerprint string, port int) (func(), error) {
	return advertiseLAN(fingerprint, port)
}

func Resolve(ctx context.Context, fingerprint string) ([]string, error) {
	return resolveLAN(ctx, fingerprint)
}

func Browse(ctx context.Context) ([]Candidate, error) { return browseLAN(ctx) }
