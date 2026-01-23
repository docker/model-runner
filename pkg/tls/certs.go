// Package tls provides TLS certificate generation and management utilities
// for the Model Runner API server.
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultCertsDir is the default directory for storing certificates.
	DefaultCertsDir = ".docker/model-runner/certs"

	// CACertFile is the filename for the CA certificate.
	CACertFile = "ca.crt"
	// CAKeyFile is the filename for the CA private key.
	CAKeyFile = "ca.key"
	// ServerCertFile is the filename for the server certificate.
	ServerCertFile = "server.crt"
	// ServerKeyFile is the filename for the server private key.
	ServerKeyFile = "server.key"

	// DefaultCertValidityDays is the default validity period for certificates.
	DefaultCertValidityDays = 365
	// DefaultCAValidityDays is the default validity period for CA certificates.
	DefaultCAValidityDays = 3650 // 10 years
)

// CertPaths holds the paths to certificate and key files.
type CertPaths struct {
	CACert     string
	CAKey      string
	ServerCert string
	ServerKey  string
}

// DefaultCertPaths returns the default certificate paths in the user's home directory.
func DefaultCertPaths() (*CertPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	certsDir := filepath.Join(homeDir, DefaultCertsDir)
	return &CertPaths{
		CACert:     filepath.Join(certsDir, CACertFile),
		CAKey:      filepath.Join(certsDir, CAKeyFile),
		ServerCert: filepath.Join(certsDir, ServerCertFile),
		ServerKey:  filepath.Join(certsDir, ServerKeyFile),
	}, nil
}

// EnsureCertificates checks for existing certificates or generates new ones.
// If certPath and keyPath are provided, they are used directly.
// Otherwise, auto-generated certificates are checked/created in the default location.
// Returns the paths to the certificate and key files.
func EnsureCertificates(certPath, keyPath string) (cert, key string, err error) {
	// If custom paths are provided, use them directly
	if certPath != "" && keyPath != "" {
		if _, err := os.Stat(certPath); err != nil {
			return "", "", fmt.Errorf("certificate file not found: %s", certPath)
		}
		if _, err := os.Stat(keyPath); err != nil {
			return "", "", fmt.Errorf("key file not found: %s", keyPath)
		}
		return certPath, keyPath, nil
	}

	// Use default paths for auto-generated certificates
	paths, err := DefaultCertPaths()
	if err != nil {
		return "", "", err
	}

	// Check if certificates already exist and are valid
	if certsExistAndValid(paths) {
		return paths.ServerCert, paths.ServerKey, nil
	}

	// Generate new certificates
	if err := GenerateCertificates(paths); err != nil {
		return "", "", fmt.Errorf("failed to generate certificates: %w", err)
	}

	return paths.ServerCert, paths.ServerKey, nil
}

// certsExistAndValid checks if certificate files exist and are not expired.
func certsExistAndValid(paths *CertPaths) bool {
	// Check if files exist
	for _, path := range []string{paths.CACert, paths.CAKey, paths.ServerCert, paths.ServerKey} {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}

	// Check if server certificate is still valid
	certPEM, err := os.ReadFile(paths.ServerCert)
	if err != nil {
		return false
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}

	// Check if certificate expires within 30 days
	if time.Until(cert.NotAfter) < 30*24*time.Hour {
		return false
	}

	return true
}

// GenerateCertificates generates a CA certificate and a server certificate signed by the CA.
func GenerateCertificates(paths *CertPaths) error {
	// Ensure the certificates directory exists
	certsDir := filepath.Dir(paths.CACert)
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return fmt.Errorf("failed to create certificates directory: %w", err)
	}

	// Generate CA certificate
	caKey, caCert, err := GenerateSelfSignedCA()
	if err != nil {
		return fmt.Errorf("failed to generate CA certificate: %w", err)
	}

	// Save CA certificate and key
	if err := saveCertAndKey(paths.CACert, paths.CAKey, caCert, caKey); err != nil {
		return fmt.Errorf("failed to save CA certificate: %w", err)
	}

	// Generate server certificate signed by CA
	serverKey, serverCert, err := GenerateServerCert(caKey, caCert)
	if err != nil {
		return fmt.Errorf("failed to generate server certificate: %w", err)
	}

	// Save server certificate and key
	if err := saveCertAndKey(paths.ServerCert, paths.ServerKey, serverCert, serverKey); err != nil {
		return fmt.Errorf("failed to save server certificate: %w", err)
	}

	return nil
}

// GenerateSelfSignedCA creates a self-signed CA certificate.
func GenerateSelfSignedCA() (*ecdsa.PrivateKey, *x509.Certificate, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{"Docker Model Runner"},
			OrganizationalUnit: []string{"Self-Signed CA"},
			CommonName:         "Docker Model Runner CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour), // Allow for clock skew
		NotAfter:              time.Now().AddDate(0, 0, DefaultCAValidityDays),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate back
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return privateKey, cert, nil
}

// GenerateServerCert creates a server certificate signed by the given CA.
func GenerateServerCert(caKey *ecdsa.PrivateKey, caCert *x509.Certificate) (*ecdsa.PrivateKey, *x509.Certificate, error) {
	// Generate private key for server
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{"Docker Model Runner"},
			OrganizationalUnit: []string{"Server"},
			CommonName:         "localhost",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour), // Allow for clock skew
		NotAfter:    time.Now().AddDate(0, 0, DefaultCertValidityDays),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost", "docker-model-runner"},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
			net.IPv4zero, // 0.0.0.0
			net.IPv6zero, // ::
		},
	}

	// Create the certificate signed by CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate back
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return privateKey, cert, nil
}

// saveCertAndKey saves a certificate and private key to PEM files.
func saveCertAndKey(certPath, keyPath string, cert *x509.Certificate, key *ecdsa.PrivateKey) error {
	// Encode and save the certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write certificate file: %w", err)
	}

	// Encode and save the private key
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	return nil
}

// LoadTLSConfig loads certificates and returns a TLS configuration for the server.
func LoadTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate and key: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// LoadClientTLSConfig loads CA certificates and returns a TLS configuration for clients.
// If caCertPath is empty, it uses the default CA certificate location.
// If skipVerify is true, certificate verification is skipped (for development only).
func LoadClientTLSConfig(caCertPath string, skipVerify bool) (*tls.Config, error) {
	if skipVerify {
		return &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Intentional for development/testing
			MinVersion:         tls.VersionTLS12,
		}, nil
	}

	// Determine CA certificate path
	if caCertPath == "" {
		paths, err := DefaultCertPaths()
		if err != nil {
			return nil, err
		}
		caCertPath = paths.CACert
	}

	// Load CA certificate
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

// GetCACertPath returns the path to the CA certificate file.
// Returns the custom path if provided, otherwise returns the default path.
func GetCACertPath(customPath string) (string, error) {
	if customPath != "" {
		return customPath, nil
	}

	paths, err := DefaultCertPaths()
	if err != nil {
		return "", err
	}
	return paths.CACert, nil
}
