package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/internal/gitinspect"
	"github.com/openmodu/oneshot/internal/worker"
)

type workspaceFlags map[string]string

func (f workspaceFlags) String() string { return fmt.Sprintf("%v", map[string]string(f)) }
func (f workspaceFlags) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("workspace must be id=/absolute/path")
	}
	f[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	return nil
}

func main() {
	workspaces := workspaceFlags{}
	listen := flag.String("listen", "127.0.0.1:9231", "listen address")
	id := flag.String("id", "worker", "stable worker ID")
	name := flag.String("name", "Oneshot Worker", "worker display name")
	token := flag.String("token", "", "shared bearer token (prefer --token-env)")
	tokenEnv := flag.String("token-env", "ONESHOT_WORKER_TOKEN", "environment variable containing the token")
	codex := flag.String("codex-binary", "", "Codex binary override")
	claude := flag.String("claude-binary", "", "Claude binary override")
	modu := flag.String("modu-binary", "", "Modu Code binary override")
	maxConcurrency := flag.Int("max-concurrency", 4, "maximum simultaneous runs (<=0 uses the default)")
	tlsCert := flag.String("tls-cert", "", "PEM server certificate for HTTPS")
	tlsKey := flag.String("tls-key", "", "PEM server private key for HTTPS")
	clientCA := flag.String("client-ca", "", "PEM CA used to require and verify mTLS client certificates")
	allowInsecureHTTP := flag.Bool("allow-insecure-http", false, "allow plain HTTP on a non-loopback listen address")
	flag.Var(workspaces, "workspace", "workspace mapping id=/absolute/path (repeatable)")
	flag.Parse()
	secret := strings.TrimSpace(*token)
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv(*tokenEnv))
	}
	if secret == "" {
		log.Fatal("worker token is required")
	}
	if len(workspaces) == 0 {
		log.Fatal("at least one --workspace mapping is required")
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
	for workspaceID, path := range workspaces {
		absolute, err := filepath.Abs(path)
		if err != nil {
			log.Fatalf("resolve workspace %s: %v", workspaceID, err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			log.Fatalf("workspace %s is not an existing directory", workspaceID)
		}
		workspaces[workspaceID] = absolute
	}
	engine := agentrun.NewEngine(agentrun.Config{CodexBinary: *codex, ClaudeBinary: *claude, ModuBinary: *modu})
	service := worker.NewServer(*id, *name, secret, workspaces, engine, *maxConcurrency)
	service.SetGitInspector(gitinspect.New(""))
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
	log.Printf("oneshot worker %s listening on %s://%s", *id, scheme, *listen)
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
		log.Printf("shutting down oneshot worker %s", *id)
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("worker shutdown: %v", err)
		}
	}
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
