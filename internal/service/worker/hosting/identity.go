package hosting

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// certificateLifetime outlives any realistic desktop install. A phone pins the
// certificate it saw while pairing, so an expiry that arrives before the user
// thinks to re-pair would break a working connection for no security gain.
const certificateLifetime = 10 * 365 * 24 * time.Hour

type identity struct {
	certificate tls.Certificate
	fingerprint string
}

// loadOrCreateCertificate keeps one self-signed certificate per install. It is
// never regenerated: the fingerprint a phone pinned during pairing is the only
// thing authenticating this machine afterwards, so a new certificate would
// silently lock out every device that had already paired.
func loadOrCreateCertificate(root, name string) (identity, error) {
	certPath := filepath.Join(root, "server-cert.pem")
	keyPath := filepath.Join(root, "server-key.pem")
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err == nil {
		return newIdentity(pair)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return identity{}, err
	}
	certPEM, keyPEM, err := generateCertificate(name)
	if err != nil {
		return identity{}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return identity{}, err
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return identity{}, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return identity{}, err
	}
	pair, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return identity{}, err
	}
	return newIdentity(pair)
}

func newIdentity(pair tls.Certificate) (identity, error) {
	if len(pair.Certificate) == 0 {
		return identity{}, fmt.Errorf("certificate contains no leaf")
	}
	digest := sha256.Sum256(pair.Certificate[0])
	return identity{certificate: pair, fingerprint: hex.EncodeToString(digest[:])}, nil
}

func generateCertificate(name string) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	subject := strings.TrimSpace(name)
	if subject == "" {
		subject = "OneCatch"
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: subject},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(certificateLifetime),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              certificateHostnames(),
		IPAddresses:           certificateIPs(),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// The names below only matter to clients that verify the usual way; a paired
// phone authenticates by fingerprint alone. They are recorded so the machine's
// own address keeps working when the address changes after issuance.
func certificateHostnames() []string {
	names := []string{"localhost"}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		names = append(names, host, host+".local")
	}
	return names
}

func certificateIPs() []net.IP {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := item.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			if ip := addressIP(address); ip != nil && !ip.IsLinkLocalUnicast() {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}
