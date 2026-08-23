package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
	"github.com/openmodu/onecatch/internal/repo/git"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/service/worker/daemon"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func Run() {
	listen := flag.String("listen", "127.0.0.1:9231", "listen address")
	id := flag.String("id", "worker", "stable worker ID")
	name := flag.String("name", "OneCatch Worker", "worker display name")
	// One --<id>-binary flag per catalogued harness, so a new harness is
	// reachable on a worker without editing this list.
	binaryFlags := make(map[string]*string, len(domainharnesses.Catalog()))
	for _, harness := range domainharnesses.Catalog() {
		binaryFlags[harness.ID] = flag.String(harness.ID+"-binary", "", harness.Name+" binary override")
	}
	maxConcurrency := flag.Int("max-concurrency", 4, "maximum simultaneous runs (<=0 uses the default)")
	dataDir := flag.String("data-dir", "~/.onecatch-worker", "persistent worker state directory")
	pair := flag.Bool("pair", false, "print a one-time desktop pairing code valid for 10 minutes")
	installService := flag.Bool("install-service", false, "install and start a per-user launchd or systemd service")
	tlsCert := flag.String("tls-cert", "", "PEM server certificate for HTTPS")
	tlsKey := flag.String("tls-key", "", "PEM server private key for HTTPS")
	clientCA := flag.String("client-ca", "", "PEM CA used to require and verify mTLS client certificates")
	allowInsecureHTTP := flag.Bool("allow-insecure-http", false, "allow plain HTTP on a non-loopback listen address")
	flag.Parse()
	stateRoot, err := expandWorkerPath(*dataDir)
	if err != nil {
		log.Fatalf("resolve worker data directory: %v", err)
	}
	if err := adoptLegacyWorkerRoot(stateRoot); err != nil {
		log.Fatalf("adopt pre-rename worker data directory: %v", err)
	}
	if (*tlsCert == "") != (*tlsKey == "") {
		log.Fatal("--tls-cert and --tls-key must be configured together")
	}
	if *clientCA != "" && *tlsCert == "" {
		log.Fatal("--client-ca requires --tls-cert and --tls-key")
	}
	if *tlsCert == "" && !*allowInsecureHTTP && !loopbackListenAddress(*listen) {
		log.Fatal("plain HTTP is restricted to loopback; configure TLS or explicitly pass --allow-insecure-http")
	}
	if *installService {
		if *pair {
			if err := requestServicePairing(stateRoot); err != nil {
				log.Fatalf("request service pairing: %v", err)
			}
		}
		binary, err := os.Executable()
		if err != nil {
			log.Fatalf("resolve worker binary: %v", err)
		}
		binary, err = filepath.Abs(binary)
		if err != nil {
			log.Fatalf("resolve worker binary: %v", err)
		}
		result, err := workerdaemon.Install(context.Background(), workerdaemon.Config{
			Binary: binary, Listen: *listen, ID: *id, Name: *name, DataDir: stateRoot,
			TLSCert: *tlsCert, TLSKey: *tlsKey, ClientCA: *clientCA,
			Binaries:       configuredBinaries(binaryFlags),
			MaxConcurrency: *maxConcurrency, AllowInsecureHTTP: *allowInsecureHTTP,
			PathEnvironment: os.Getenv("PATH"),
		})
		if err != nil {
			log.Fatalf("install worker service: %v", err)
		}
		log.Printf("worker service installed: %s", result.ServiceFile)
		log.Printf("follow the startup log for the pairing code: %s", result.LogHint)
		return
	}
	secret, tokenCreated, err := loadOrCreateWorkerToken(stateRoot)
	if err != nil {
		log.Fatalf("load worker token: %v", err)
	}
	pairRequested, err := consumeServicePairingRequest(stateRoot)
	if err != nil {
		log.Fatalf("load service pairing request: %v", err)
	}
	engine := agentrun.NewEngine(agentrun.Config{
		Binaries: configuredBinaries(binaryFlags),
		// DeepSeek Harness recovers its event stream by reading its own session
		// log, so it needs a directory this worker owns.
		DshSessionRoot: filepath.Join(stateRoot, "harnesses", "dsh", "sessions"),
	})
	service := worker.NewServer(*id, *name, secret, nil, engine, *maxConcurrency)
	if err := service.SetWorkspaceRegistry(context.Background(), worker.NewWorkspaceRegistry(filepath.Join(stateRoot, "workspaces.json"))); err != nil {
		log.Fatalf("load worker workspace mappings: %v", err)
	}
	service.SetGitInspector(gitrepo.New(""))
	if *pair || pairRequested || tokenCreated {
		code, err := newPairingCode()
		if err != nil {
			log.Fatalf("create pairing code: %v", err)
		}
		service.EnablePairing(code, time.Now().Add(10*time.Minute), *tlsCert == "" && (*allowInsecureHTTP || loopbackListenAddress(*listen)))
		log.Printf("desktop pairing code: %s (valid for 10 minutes, one use)", code)
	}
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: *listen, Handler: service.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: time.Minute,
		WriteTimeout: worker.MaxRunDuration + 2*time.Minute, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return runCtx },
	}
	if *tlsCert != "" {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if *clientCA != "" {
		pem, err := os.ReadFile(*clientCA)
		if err != nil {
			log.Fatalf("read client CA: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			log.Fatal("client CA file contains no certificates")
		}
		server.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
		server.TLSConfig.ClientCAs = pool
	}
	scheme := "http"
	if *tlsCert != "" {
		scheme = "https"
		fingerprint, err := certificateFingerprint(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("load TLS certificate: %v", err)
		}
		log.Printf("server certificate SHA-256: %s", fingerprint)
	}
	log.Printf("onecatch worker %s listening on %s://%s", *id, scheme, *listen)
	serveErrors := make(chan error, 1)
	go func() {
		if *tlsCert != "" {
			serveErrors <- server.ListenAndServeTLS(*tlsCert, *tlsKey)
			return
		}
		serveErrors <- server.ListenAndServe()
	}()
	select {
	case serveErr := <-serveErrors:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatal(serveErr)
		}
	case <-runCtx.Done():
		log.Printf("shutting down onecatch worker %s", *id)
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("worker shutdown: %v", err)
		}
	}
}

func requestServicePairing(stateRoot string) error {
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return err
	}
	path := filepath.Join(stateRoot, "pair-once")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func consumeServicePairingRequest(stateRoot string) (bool, error) {
	path := filepath.Join(stateRoot, "pair-once")
	if err := os.Remove(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func loadOrCreateWorkerToken(stateRoot string) (string, bool, error) {
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return "", false, err
	}
	path := filepath.Join(stateRoot, "token")
	value, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(value))
		if token == "" {
			return "", false, fmt.Errorf("%s is empty", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", false, err
		}
		return token, false, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", false, err
	}
	token := hex.EncodeToString(buffer)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return loadOrCreateWorkerToken(stateRoot)
	}
	if err != nil {
		return "", false, err
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", false, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", false, err
	}
	return token, true, nil
}

func newPairingCode() (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}
	return string(buffer[:4]) + "-" + string(buffer[4:]), nil
}

// adoptLegacyWorkerRoot moves a pre-rename worker state directory into place on
// first start after the rename. The persistent worker token lives here, so
// losing it would silently invalidate every desktop that had already paired.
// Only the default location is adopted, and only when nothing would be
// overwritten — an explicit --data-dir is always left alone.
func adoptLegacyWorkerRoot(stateRoot string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if stateRoot != filepath.Join(home, ".onecatch-worker") {
		return nil
	}
	if _, err := os.Stat(stateRoot); err == nil || !os.IsNotExist(err) {
		return nil
	}
	legacy := filepath.Join(home, ".oneshot-worker")
	info, err := os.Stat(legacy)
	if err != nil || !info.IsDir() {
		return nil
	}
	if err := os.Rename(legacy, stateRoot); err != nil {
		return fmt.Errorf("move %s to %s (your data is untouched; move it manually and restart): %w", legacy, stateRoot, err)
	}
	return nil
}

func expandWorkerPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	return filepath.Abs(value)
}

func loopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func certificateFingerprint(certFile, keyFile string) (string, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return "", err
	}
	if len(pair.Certificate) == 0 {
		return "", fmt.Errorf("certificate chain is empty")
	}
	sum := sha256.Sum256(pair.Certificate[0])
	return fmt.Sprintf("%x", sum[:]), nil
}

// configuredBinaries collects the harness executable overrides the operator
// actually set, dropping the empty defaults.
func configuredBinaries(flags map[string]*string) map[string]string {
	binaries := make(map[string]string, len(flags))
	for id, value := range flags {
		if strings.TrimSpace(*value) != "" {
			binaries[id] = *value
		}
	}
	return binaries
}
