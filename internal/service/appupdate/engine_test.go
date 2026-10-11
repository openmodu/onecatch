package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNativeUpdaterChecksDownloadsAndAuthenticates(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("signed native application")
	digest := sha256.Sum256(payload)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(private, digest[:]))
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			var address string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/update.zip" {
					if corrupt {
						_, _ = w.Write([]byte("tampered native app!!!!!"))
					} else {
						_, _ = w.Write(payload)
					}
					return
				}
				fmt.Fprintf(w, `<rss xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle"><channel><item><title>OneCatch</title><sparkle:version>2.0.0</sparkle:version><sparkle:os>%s</sparkle:os><enclosure url="%s/update.zip" sparkle:edSignature="%s" /></item></channel></rss>`, runtime.GOOS, address, signature)
			}))
			defer server.Close()
			address = server.URL
			engine := &updateEngine{url: address, currentVersion: "1.0.0", key: public}
			release, err := engine.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if release == nil || release.Version != "2.0.0" || engine.State() != "available" {
				t.Fatalf("release = %#v", release)
			}
			err = engine.DownloadAndInstall(context.Background())
			if corrupt {
				if err == nil || !strings.Contains(err.Error(), "signature") || engine.DownloadedPath() != "" {
					t.Fatalf("tampering accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer discardTemporaryDownload(engine.DownloadedPath())
			data, err := os.ReadFile(engine.DownloadedPath())
			if err != nil || string(data) != string(payload) {
				t.Fatalf("download = %q, %v", data, err)
			}
			cached, err := persistCachedUpdate(t.TempDir(), release, engine.DownloadedPath())
			if err != nil || cached == nil {
				t.Fatalf("cache: %v", err)
			}
			engine.currentVersion = "2.0.0"
			if release, err := engine.Check(context.Background()); err != nil || release != nil || engine.State() != "up-to-date" {
				t.Fatalf("same release offered again: %v %v", release, err)
			}
		})
	}
}
