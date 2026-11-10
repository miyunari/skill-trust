package keypair

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

// TestLoadFromPEMPKCS1RSA covers the documented PKCS#1 ("RSA PRIVATE KEY")
// PEM form, which previously failed despite being advertised.
func TestLoadFromPEMPKCS1RSA(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	kp, err := LoadFromPEM(pemBytes)
	if err != nil {
		t.Fatalf("LoadFromPEM(PKCS1 RSA): %v", err)
	}
	if kp == nil {
		t.Fatal("LoadFromPEM returned nil keypair")
	}
}

// TestLoadFromPEMRejectsUnsupportedRSAModulus covers RSA keys whose
// modulus does not map to a bundle algorithm identifier: they must be
// rejected, not silently mislabeled (previously anything below 3072 bits
// was labeled as the 2048 identifier).
func TestLoadFromPEMRejectsUnsupportedRSAModulus(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	_, err = LoadFromPEM(pemBytes)
	if err == nil {
		t.Fatal("expected 1024-bit RSA key to be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported RSA modulus") {
		t.Errorf("err = %v, want modulus-size rejection", err)
	}
}
