package landiscovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInstance(t *testing.T) {
	want := "onecatch-" + strings.Repeat("ab", 16)
	for _, pin := range []string{strings.Repeat("ab", 32), strings.TrimSuffix(strings.Repeat("AB:", 32), ":")} {
		got, err := Instance(pin)
		if err != nil || got != want {
			t.Fatalf("got %q %v", got, err)
		}
	}
	for _, pin := range []string{"", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		if _, err := Instance(pin); err == nil {
			t.Fatal("accepted invalid fingerprint")
		}
	}
}

func TestEndpoint(t *testing.T) {
	for ip, want := range map[string]string{
		"192.168.1.2": "https://192.168.1.2:9232",
		"fe80::1%en0": "https://[fe80::1%25en0]:9232",
		"fd00::1":     "https://[fd00::1]:9232",
		"127.0.0.1":   "", "::1": "", "0.0.0.0": "", "::": "", "224.0.0.251": "", "bad": "",
	} {
		if got := endpoint(ip, 9232); got != want {
			t.Errorf("%s: %q", ip, got)
		}
	}
	if endpoint("192.168.1.2", 0) != "" || endpoint("192.168.1.2", 65536) != "" {
		t.Fatal("invalid port accepted")
	}
}

func TestResolveCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Resolve(ctx, strings.Repeat("ab", 32))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

// Opt-in because multicast and OS local-network permission are not available
// on every CI runner. Exercise actual registration and discovery together.
func TestLANDiscovery(t *testing.T) {
	if os.Getenv("ONECATCH_TEST_MDNS") != "1" {
		t.Skip("set ONECATCH_TEST_MDNS=1 on a LAN-connected host")
	}
	pin := randomFingerprint(t)
	stop, err := Advertise(pin, 19232)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	addresses, err := Resolve(ctx, pin)
	if err != nil || len(addresses) == 0 {
		t.Fatalf("addresses=%v err=%v", addresses, err)
	}
	t.Logf("discovered %v", addresses)
}

func randomFingerprint(t *testing.T) string {
	t.Helper()
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw[:])
}

func TestBrowseCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Browse(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestBrowseLAN(t *testing.T) {
	if os.Getenv("ONECATCH_TEST_MDNS") != "1" {
		t.Skip("requires LAN multicast")
	}
	pin := randomFingerprint(t)
	stop, err := Advertise(pin, 19233)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	items, err := Browse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instance, _ := Instance(pin)
	for _, item := range items {
		if item.ID == instance && item.Name != "" && len(item.Addresses) > 0 {
			return
		}
	}
	t.Fatalf("advertised service was not discovered: %+v", items)
}
