//go:build darwin && cgo

package landiscovery

import (
	"context"
	"os"
	"testing"
)

func TestBonjourInterop(t *testing.T) {
	if os.Getenv("ONECATCH_TEST_MDNS") != "1" {
		t.Skip("requires LAN multicast")
	}
	for _, test := range []struct {
		name      string
		advertise func(string, int) (func(), error)
		resolve   func(context.Context, string) ([]string, error)
	}{
		{"BonjourToGo", Advertise, resolveLAN},
		{"GoToBonjour", advertiseLAN, Resolve},
		{"GoToGo", advertiseLAN, resolveLAN},
	} {
		t.Run(test.name, func(t *testing.T) {
			pin := randomFingerprint(t)
			stop, err := test.advertise(pin, 19232)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			addresses, err := test.resolve(context.Background(), pin)
			if err != nil || len(addresses) == 0 {
				t.Fatalf("%v %v", addresses, err)
			}
			t.Log(addresses)
		})
	}
}
