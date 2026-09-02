package appupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"golang.org/x/mod/semver"
)

const (
	updateCacheSchema = 1
	readyCacheName    = "ready"
	cacheManifestName = "manifest.json"
)

type cacheManifest struct {
	Schema        int             `json:"schema"`
	Release       updater.Release `json:"release"`
	Platform      string          `json:"platform"`
	Architecture  string          `json:"architecture"`
	Payload       string          `json:"payload"`
	PayloadSHA256 string          `json:"payloadSha256"`
	PayloadSize   int64           `json:"payloadSize"`
	VerifiedAt    time.Time       `json:"verifiedAt"`
}

type cachedUpdate struct {
	manifest cacheManifest
	payload  string
}

func updateCacheRoot(dataRoot string) (string, error) {
	if strings.TrimSpace(dataRoot) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve update cache root: %w", err)
		}
		dataRoot = filepath.Join(home, ".onecatch")
	}
	abs, err := filepath.Abs(dataRoot)
	if err != nil {
		return "", fmt.Errorf("resolve update cache root: %w", err)
	}
	return filepath.Join(filepath.Clean(abs), "updates"), nil
}

func validateSignedRelease(release *updater.Release) error {
	if release == nil {
		return errors.New("app update: check for an update before downloading")
	}
	if !validCacheName(release.Version) {
		return errors.New("app update: release has an invalid version")
	}
	verification := release.Verification
	if verification == nil || verification.SignatureAlgo != "ed25519" || len(verification.Signature) != ed25519.SignatureSize {
		return errors.New("app update: release is missing a valid Ed25519 signature")
	}
	return nil
}

func persistCachedUpdate(root string, release *updater.Release, staged string) (*cachedUpdate, error) {
	if err := validateSignedRelease(release); err != nil {
		return nil, err
	}
	if strings.TrimSpace(staged) == "" {
		return nil, errors.New("app update: verified artifact is missing")
	}
	payloadName := filepath.Base(filepath.Clean(staged))
	if !validCacheName(payloadName) {
		return nil, errors.New("app update: verified artifact has an invalid name")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("app update: create cache directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("app update: secure cache directory: %w", err)
	}
	pending, err := os.MkdirTemp(root, ".pending-")
	if err != nil {
		return nil, fmt.Errorf("app update: create pending cache: %w", err)
	}
	keepPending := false
	defer func() {
		if !keepPending {
			_ = os.RemoveAll(pending)
		}
	}()

	destination := filepath.Join(pending, payloadName)
	if err := copyCachedPayload(staged, destination); err != nil {
		return nil, fmt.Errorf("app update: persist verified artifact: %w", err)
	}
	digest, size, err := hashCachedPayload(destination)
	if err != nil {
		return nil, fmt.Errorf("app update: verify persisted artifact: %w", err)
	}
	manifest := cacheManifest{
		Schema:        updateCacheSchema,
		Release:       *release,
		Platform:      runtime.GOOS,
		Architecture:  runtime.GOARCH,
		Payload:       payloadName,
		PayloadSHA256: digest,
		PayloadSize:   size,
		VerifiedAt:    time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("app update: encode cache manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(pending, cacheManifestName), encoded, 0o600); err != nil {
		return nil, fmt.Errorf("app update: write cache manifest: %w", err)
	}

	ready := filepath.Join(root, readyCacheName)
	previous := filepath.Join(root, ".previous")
	_ = os.RemoveAll(previous)
	if _, err := os.Stat(ready); err == nil {
		if err := os.Rename(ready, previous); err != nil {
			return nil, fmt.Errorf("app update: rotate previous cache: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("app update: inspect previous cache: %w", err)
	}
	if err := os.Rename(pending, ready); err != nil {
		_ = os.Rename(previous, ready)
		return nil, fmt.Errorf("app update: activate cache: %w", err)
	}
	keepPending = true
	_ = os.RemoveAll(previous)
	return &cachedUpdate{manifest: manifest, payload: filepath.Join(ready, payloadName)}, nil
}

func restoreCachedUpdate(root, currentVersion string, publicKey []byte) (*cachedUpdate, error) {
	ready := filepath.Join(root, readyCacheName)
	manifestPath := filepath.Join(ready, cacheManifestName)
	file, err := os.Open(manifestPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("app update: open cache manifest: %w", err)
	}
	defer func() { _ = file.Close() }()
	var manifest cacheManifest
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("app update: decode cache manifest: %w", err)
	}
	if manifest.Schema != updateCacheSchema || manifest.Platform != runtime.GOOS || manifest.Architecture != runtime.GOARCH {
		return nil, errors.New("app update: cached artifact does not match this application")
	}
	if err := validateSignedRelease(&manifest.Release); err != nil {
		return nil, err
	}
	if !versionIsNewer(manifest.Release.Version, currentVersion) {
		if err := removeCachedUpdate(root); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if manifest.Payload == "" || manifest.Payload != filepath.Base(manifest.Payload) || !validCacheName(manifest.Payload) {
		return nil, errors.New("app update: cache manifest has an unsafe payload path")
	}
	payload := filepath.Join(ready, manifest.Payload)
	digest, size, err := hashCachedPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("app update: inspect cached artifact: %w", err)
	}
	if digest != manifest.PayloadSHA256 || size != manifest.PayloadSize {
		return nil, errors.New("app update: cached artifact failed integrity validation")
	}
	info, err := os.Lstat(payload)
	if err != nil {
		return nil, fmt.Errorf("app update: inspect cached artifact: %w", err)
	}
	// Windows installers and Linux AppImages remain byte-for-byte identical
	// to the signed enclosure, so re-check the publisher signature as well as
	// the local manifest hash. macOS enclosures are ZIPs that Wails verifies
	// before extracting the .app; the deterministic tree hash above protects
	// that verified extracted payload across launches.
	if !info.IsDir() {
		digestBytes, decodeErr := hex.DecodeString(digest)
		verification := manifest.Release.Verification
		if decodeErr != nil || len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(publicKey), digestBytes, verification.Signature) {
			return nil, errors.New("app update: cached artifact failed publisher signature validation")
		}
	}
	return &cachedUpdate{manifest: manifest, payload: payload}, nil
}

func removeCachedUpdate(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("app update: refusing to remove an empty cache path")
	}
	if err := os.RemoveAll(filepath.Join(filepath.Clean(root), readyCacheName)); err != nil {
		return fmt.Errorf("app update: remove cached artifact: %w", err)
	}
	return nil
}

func validCacheName(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 180 {
		return false
	}
	return !strings.ContainsAny(value, "/\\\x00")
}

func versionIsNewer(candidate, current string) bool {
	candidate = "v" + strings.TrimPrefix(strings.TrimSpace(candidate), "v")
	current = "v" + strings.TrimPrefix(strings.TrimSpace(current), "v")
	return semver.IsValid(candidate) && semver.IsValid(current) && semver.Compare(candidate, current) > 0
}

func copyCachedPayload(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(target, destination)
	case info.IsDir():
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyCachedPayload(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(destination, info.Mode().Perm())
	case info.Mode().IsRegular():
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Sync(); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	default:
		return fmt.Errorf("unsupported payload entry %s (%s)", source, info.Mode())
	}
}

func hashCachedPayload(path string) (string, int64, error) {
	hasher := sha256.New()
	var total int64
	rootInfo, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !rootInfo.IsDir() {
		if !rootInfo.Mode().IsRegular() {
			return "", 0, fmt.Errorf("unsupported payload entry %s (%s)", path, rootInfo.Mode())
		}
		file, err := os.Open(path)
		if err != nil {
			return "", 0, err
		}
		written, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", 0, copyErr
		}
		if closeErr != nil {
			return "", 0, closeErr
		}
		return hex.EncodeToString(hasher.Sum(nil)), written, nil
	}
	err = filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(path, candidate)
		if err != nil {
			return err
		}
		return hashCacheEntry(hasher, candidate, filepath.ToSlash(relative), info, &total)
	})
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), total, nil
}

func hashCacheEntry(hasher hash.Hash, path, relative string, info fs.FileInfo, total *int64) error {
	kind := byte('f')
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		kind = 'l'
	case info.IsDir():
		kind = 'd'
	case !info.Mode().IsRegular():
		return fmt.Errorf("unsupported payload entry %s (%s)", path, info.Mode())
	}
	_, _ = fmt.Fprintf(hasher, "%c\x00%s\x00%o\x00%d\x00", kind, relative, info.Mode().Perm(), info.Size())
	if kind == 'l' {
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(hasher, target)
		*total += int64(len(target))
		return nil
	}
	if kind == 'd' {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(hasher, file)
	closeErr := file.Close()
	*total += written
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
