package appupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func signedFileRelease(t *testing.T, version string, payload []byte) (*updater.Release, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return &updater.Release{
		Version: version,
		Name:    "OneCatch " + version,
		Verification: &updater.Verification{
			SignatureAlgo: "ed25519",
			Signature:     ed25519.Sign(privateKey, digest[:]),
		},
	}, publicKey
}

func TestCachedUpdateSurvivesRestartAndRevalidatesPublisherSignature(t *testing.T) {
	root := filepath.Join(t.TempDir(), "updates")
	payload := []byte("verified update payload")
	release, publicKey := signedFileRelease(t, "2.0.0", payload)
	staged := filepath.Join(t.TempDir(), "OneCatch-2.0.0.AppImage")
	if err := os.WriteFile(staged, payload, 0o755); err != nil {
		t.Fatal(err)
	}

	cached, err := persistCachedUpdate(root, release, staged)
	if err != nil {
		t.Fatal(err)
	}
	if cached.payload != filepath.Join(root, readyCacheName, filepath.Base(staged)) {
		t.Fatalf("cached payload = %q", cached.payload)
	}
	restored, err := restoreCachedUpdate(root, "1.0.0", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if restored == nil || restored.manifest.Release.Version != "2.0.0" {
		t.Fatalf("restored update = %+v", restored)
	}
}

func TestCachedUpdateRejectsModifiedPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "updates")
	payload := []byte("verified update payload")
	release, publicKey := signedFileRelease(t, "2.0.0", payload)
	staged := filepath.Join(t.TempDir(), "OneCatch-2.0.0.exe")
	if err := os.WriteFile(staged, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	cached, err := persistCachedUpdate(root, release, staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached.payload, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCachedUpdate(root, "1.0.0", publicKey); err == nil {
		t.Fatal("expected modified cached payload to be rejected")
	}
}

func TestCachedFileRechecksPublisherSignatureWhenManifestIsRewritten(t *testing.T) {
	root := filepath.Join(t.TempDir(), "updates")
	payload := []byte("verified update payload")
	release, publicKey := signedFileRelease(t, "2.0.0", payload)
	staged := filepath.Join(t.TempDir(), "OneCatch-2.0.0.exe")
	if err := os.WriteFile(staged, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	cached, err := persistCachedUpdate(root, release, staged)
	if err != nil {
		t.Fatal(err)
	}
	forged := []byte("untrusted update payload")
	if err := os.WriteFile(cached.payload, forged, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(forged)
	cached.manifest.PayloadSHA256 = fmt.Sprintf("%x", digest)
	cached.manifest.PayloadSize = int64(len(forged))
	encoded, err := json.Marshal(cached.manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, readyCacheName, cacheManifestName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCachedUpdate(root, "1.0.0", publicKey); err == nil {
		t.Fatal("expected forged payload and manifest to fail publisher signature validation")
	}
}

func TestCachedDirectoryDetectsChangesAndPreservesSymlinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "updates")
	staged := filepath.Join(t.TempDir(), "OneCatch.app")
	binary := filepath.Join(staged, "Contents", "MacOS", "OneCatch")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("bundle binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("MacOS/OneCatch", filepath.Join(staged, "Contents", "Current")); err != nil {
		t.Fatal(err)
	}
	release, publicKey := signedFileRelease(t, "2.0.0", []byte("signed source archive"))
	cached, err := persistCachedUpdate(root, release, staged)
	if err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(cached.payload, "Contents", "Current")); err != nil || target != "MacOS/OneCatch" {
		t.Fatalf("cached symlink = %q, %v", target, err)
	}
	if _, err := restoreCachedUpdate(root, "1.0.0", publicKey); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cached.payload, "Contents", "MacOS", "OneCatch"), []byte("changed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCachedUpdate(root, "1.0.0", publicKey); err == nil {
		t.Fatal("expected changed app bundle to be rejected")
	}
}

func TestCachedUpdateIsRemovedAfterVersionBecomesCurrent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "updates")
	payload := []byte("verified update payload")
	release, publicKey := signedFileRelease(t, "2.0.0", payload)
	staged := filepath.Join(t.TempDir(), "OneCatch.exe")
	if err := os.WriteFile(staged, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := persistCachedUpdate(root, release, staged); err != nil {
		t.Fatal(err)
	}
	restored, err := restoreCachedUpdate(root, "2.0.0", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if restored != nil {
		t.Fatalf("current-version cache was restored: %+v", restored)
	}
	if _, err := os.Stat(filepath.Join(root, readyCacheName)); !os.IsNotExist(err) {
		t.Fatalf("current-version cache still exists: %v", err)
	}
}

func TestUnsignedReleaseCannotEnterPersistentCache(t *testing.T) {
	staged := filepath.Join(t.TempDir(), "update.bin")
	if err := os.WriteFile(staged, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := persistCachedUpdate(filepath.Join(t.TempDir(), "updates"), &updater.Release{Version: "2.0.0"}, staged)
	if err == nil {
		t.Fatal("expected unsigned release to be rejected")
	}
}
