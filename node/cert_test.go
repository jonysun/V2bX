package node

import (
	"path/filepath"
	"testing"
)

func Test_generateSelfSslCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "1.pem")
	keyPath := filepath.Join(dir, "1.key")
	if err := generateSelfSslCertificate("domain.com", certPath, keyPath); err != nil {
		t.Fatal(err)
	}
}
