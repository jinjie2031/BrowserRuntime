//go:build windows

package network

import (
	"bytes"
	"testing"
)

func TestDPAPICurrentUserRoundTrip(t *testing.T) {
	protector := newDPAPIProtector()
	plain := []byte(randomTestSecret(t))
	cipher, err := protector.Protect(plain)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if bytes.Contains(cipher, plain) {
		t.Fatal("Protect() returned plaintext")
	}
	got, err := protector.Unprotect(cipher)
	if err != nil {
		t.Fatalf("Unprotect() error = %v", err)
	}
	defer zeroBytes(got)
	if !bytes.Equal(got, plain) {
		t.Fatal("Unprotect() did not round-trip")
	}
}
