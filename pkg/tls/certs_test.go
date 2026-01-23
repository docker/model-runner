package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testCertPaths(t *testing.T) *CertPaths {
	t.Helper()
	tmpDir := t.TempDir()
	return &CertPaths{
		CACert:     filepath.Join(tmpDir, CACertFile),
		CAKey:      filepath.Join(tmpDir, CAKeyFile),
		ServerCert: filepath.Join(tmpDir, ServerCertFile),
		ServerKey:  filepath.Join(tmpDir, ServerKeyFile),
	}
}

func setupTestCerts(t *testing.T) *CertPaths {
	t.Helper()
	paths := testCertPaths(t)
	if err := GenerateCertificates(paths); err != nil {
		t.Fatalf("GenerateCertificates() error = %v", err)
	}
	return paths
}

func TestGenerateSelfSignedCA(t *testing.T) {
	key, cert, err := GenerateSelfSignedCA()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA() error = %v", err)
	}

	if key == nil {
		t.Error("GenerateSelfSignedCA() returned nil key")
	}

	if cert == nil {
		t.Error("GenerateSelfSignedCA() returned nil cert")
		return // Return early to avoid nil dereference
	}

	// Verify the certificate is a CA
	if !cert.IsCA {
		t.Error("Generated certificate is not a CA")
	}

	// Verify the certificate has proper key usage
	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("CA certificate missing KeyUsageCertSign")
	}

	// Verify validity period
	if cert.NotAfter.Before(time.Now().AddDate(0, 0, DefaultCAValidityDays-1)) {
		t.Error("CA certificate validity period is too short")
	}
}

func TestGenerateServerCert(t *testing.T) {
	// First generate a CA
	caKey, caCert, err := GenerateSelfSignedCA()
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA() error = %v", err)
	}

	// Generate server certificate
	serverKey, serverCert, err := GenerateServerCert(caKey, caCert)
	if err != nil {
		t.Fatalf("GenerateServerCert() error = %v", err)
	}

	if serverKey == nil {
		t.Error("GenerateServerCert() returned nil key")
	}

	if serverCert == nil {
		t.Error("GenerateServerCert() returned nil cert")
		return // Return early to avoid nil dereference
	}

	// Verify the certificate is NOT a CA
	if serverCert.IsCA {
		t.Error("Server certificate should not be a CA")
	}

	// Verify server authentication extended key usage
	hasServerAuth := false
	for _, usage := range serverCert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
			break
		}
	}
	if !hasServerAuth {
		t.Error("Server certificate missing ExtKeyUsageServerAuth")
	}

	// Verify DNS names
	expectedDNS := []string{"localhost", "docker-model-runner"}
	for _, expected := range expectedDNS {
		found := false
		for _, dns := range serverCert.DNSNames {
			if dns == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Server certificate missing DNS name: %s", expected)
		}
	}

	// Verify the certificate was signed by the CA
	err = serverCert.CheckSignatureFrom(caCert)
	if err != nil {
		t.Errorf("Server certificate not signed by CA: %v", err)
	}
}

func TestGenerateCertificates(t *testing.T) {
	paths := setupTestCerts(t)

	// Verify all files exist
	for _, path := range []string{paths.CACert, paths.CAKey, paths.ServerCert, paths.ServerKey} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not created: %s", path)
		}
	}

	// Verify certificate permissions
	if info, err := os.Stat(paths.CAKey); err == nil {
		if info.Mode().Perm() != 0600 {
			t.Errorf("CA key has wrong permissions: %v, expected 0600", info.Mode().Perm())
		}
	}

	if info, err := os.Stat(paths.ServerKey); err == nil {
		if info.Mode().Perm() != 0600 {
			t.Errorf("Server key has wrong permissions: %v, expected 0600", info.Mode().Perm())
		}
	}
}

func TestLoadTLSConfig(t *testing.T) {
	paths := setupTestCerts(t)

	// Load TLS config
	tlsConfig, err := LoadTLSConfig(paths.ServerCert, paths.ServerKey)
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	if tlsConfig == nil {
		t.Error("LoadTLSConfig() returned nil config")
		return // Return early to avoid nil dereference
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(tlsConfig.Certificates))
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Error("TLS minimum version should be TLS 1.2")
	}
}

func TestLoadClientTLSConfig(t *testing.T) {
	paths := setupTestCerts(t)

	// Test loading with CA cert
	tlsConfig, err := LoadClientTLSConfig(paths.CACert, false)
	if err != nil {
		t.Fatalf("LoadClientTLSConfig() error = %v", err)
	}

	if tlsConfig == nil {
		t.Error("LoadClientTLSConfig() returned nil config")
		return // Return early to avoid nil dereference
	}

	if tlsConfig.RootCAs == nil {
		t.Error("LoadClientTLSConfig() should have RootCAs set")
	}

	if tlsConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false when skipVerify=false")
	}

	// Test with skipVerify=true
	tlsConfig, err = LoadClientTLSConfig("", true)
	if err != nil {
		t.Fatalf("LoadClientTLSConfig() with skipVerify error = %v", err)
	}

	if !tlsConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true when skipVerify=true")
	}
}

func TestLoadClientTLSConfig_CAErrors(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("non-existent CA path", func(t *testing.T) {
		paths := &CertPaths{
			CACert: filepath.Join(tmpDir, "does-not-exist-ca.pem"),
		}

		tlsConfig, err := LoadClientTLSConfig(paths.CACert, false)
		if err == nil {
			t.Fatalf("expected error when loading client TLS config with non-existent CA path, got nil")
		}
		if tlsConfig != nil {
			t.Fatalf("expected nil TLS config on CA load error, got non-nil")
		}
	})

	t.Run("invalid CA PEM", func(t *testing.T) {
		caPath := filepath.Join(tmpDir, "invalid-ca.pem")
		if err := os.WriteFile(caPath, []byte("this is not a valid PEM"), 0o600); err != nil {
			t.Fatalf("failed to write invalid CA file: %v", err)
		}

		tlsConfig, err := LoadClientTLSConfig(caPath, false)
		if err == nil {
			t.Fatalf("expected error when loading client TLS config with invalid CA PEM, got nil")
		}
		if tlsConfig != nil {
			t.Fatalf("expected nil TLS config on CA load error, got non-nil")
		}
	})
}

func TestEnsureCertificates_CustomPaths(t *testing.T) {
	paths := setupTestCerts(t)

	// Test with custom paths
	cert, key, err := EnsureCertificates(paths.ServerCert, paths.ServerKey)
	if err != nil {
		t.Fatalf("EnsureCertificates() error = %v", err)
	}

	if cert != paths.ServerCert {
		t.Errorf("Expected cert path %s, got %s", paths.ServerCert, cert)
	}

	if key != paths.ServerKey {
		t.Errorf("Expected key path %s, got %s", paths.ServerKey, key)
	}
}

func TestEnsureCertificates_MissingFile(t *testing.T) {
	_, _, err := EnsureCertificates("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("EnsureCertificates() should return error for missing files")
	}
}

func TestCertsExistAndValid(t *testing.T) {
	paths := testCertPaths(t)

	// Should return false when files don't exist
	if certsExistAndValid(paths) {
		t.Error("certsExistAndValid() should return false for non-existent files")
	}

	// Generate certs
	if err := GenerateCertificates(paths); err != nil {
		t.Fatalf("GenerateCertificates() error = %v", err)
	}

	// Should return true for valid certs
	if !certsExistAndValid(paths) {
		t.Error("certsExistAndValid() should return true for valid certs")
	}

	t.Run("near-expiry certs treated as invalid", func(t *testing.T) {
		// Load CA certificate
		caCertPEM, err := os.ReadFile(paths.CACert)
		if err != nil {
			t.Fatalf("failed to read CA cert: %v", err)
		}
		caCertBlock, _ := pem.Decode(caCertPEM)
		if caCertBlock == nil {
			t.Fatal("failed to decode CA cert PEM")
		}
		caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
		if err != nil {
			t.Fatalf("failed to parse CA cert: %v", err)
		}

		// Load CA private key
		caKeyPEM, err := os.ReadFile(paths.CAKey)
		if err != nil {
			t.Fatalf("failed to read CA key: %v", err)
		}
		caKeyBlock, _ := pem.Decode(caKeyPEM)
		if caKeyBlock == nil {
			t.Fatal("failed to decode CA key PEM")
		}
		caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
		if err != nil {
			// Try parsing as PKCS#8
			caKeyI, err := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
			if err != nil {
				t.Fatalf("failed to parse CA key: %v", err)
			}
			var ok bool
			caKey, ok = caKeyI.(*ecdsa.PrivateKey)
			if !ok {
				t.Fatal("not an ECDSA private key")
			}
		}

		// Create a server certificate that expires in 15 days
		serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate server key: %v", err)
		}

		template := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(15 * 24 * time.Hour), // within 30 days, should be treated as invalid
			KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			DNSNames:     []string{"localhost", "docker-model-runner"},
		}

		derBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &serverKey.PublicKey, caKey)
		if err != nil {
			t.Fatalf("failed to create near-expiry server cert: %v", err)
		}

		// Overwrite server cert and key on disk with the near-expiry ones
		serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
		if err := os.WriteFile(paths.ServerCert, serverCertPEM, 0o600); err != nil {
			t.Fatalf("failed to write near-expiry server cert: %v", err)
		}

		serverKeyBytes, err := x509.MarshalECPrivateKey(serverKey)
		if err != nil {
			t.Fatalf("failed to marshal server key: %v", err)
		}
		serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyBytes})
		if err := os.WriteFile(paths.ServerKey, serverKeyPEM, 0o600); err != nil {
			t.Fatalf("failed to write near-expiry server key: %v", err)
		}

		// Now certsExistAndValid should treat these near-expiry certs as invalid
		if certsExistAndValid(paths) {
			t.Error("certsExistAndValid() should return false for near-expiry certificates")
		}
	})
}

func TestTLSServerIntegration(t *testing.T) {
	paths := setupTestCerts(t)

	// Load server TLS config
	serverTLSConfig, err := LoadTLSConfig(paths.ServerCert, paths.ServerKey)
	if err != nil {
		t.Fatalf("LoadTLSConfig() error = %v", err)
	}

	// Create a test server
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	server.TLS = serverTLSConfig
	server.StartTLS()
	defer server.Close()

	// Load client TLS config
	clientTLSConfig, err := LoadClientTLSConfig(paths.CACert, false)
	if err != nil {
		t.Fatalf("LoadClientTLSConfig() error = %v", err)
	}

	// Create HTTP client with TLS config
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}

	// Make a request
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCertificatePEMEncoding(t *testing.T) {
	paths := setupTestCerts(t)

	// Read and verify PEM encoding of certificate
	certPEM, err := os.ReadFile(paths.ServerCert)
	if err != nil {
		t.Fatalf("Failed to read cert file: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Error("Failed to decode certificate PEM")
		return // Return early to avoid nil dereference
	}
	if block.Type != "CERTIFICATE" {
		t.Errorf("Expected PEM type CERTIFICATE, got %s", block.Type)
	}

	// Read and verify PEM encoding of key
	keyPEM, err := os.ReadFile(paths.ServerKey)
	if err != nil {
		t.Fatalf("Failed to read key file: %v", err)
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		t.Error("Failed to decode key PEM")
		return // Return early to avoid nil dereference
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("Expected PEM type EC PRIVATE KEY, got %s", block.Type)
	}
}
