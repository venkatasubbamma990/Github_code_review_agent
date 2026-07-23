package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(body, sig, secret) {
		t.Fatal("expected valid signature")
	}
	if verifyGitHubSignature(body, "sha256=deadbeef", secret) {
		t.Fatal("expected invalid signature to fail")
	}
	if verifyGitHubSignature(body, "", secret) {
		t.Fatal("empty signature should fail")
	}
	if verifyGitHubSignature(body, "md5=abc", secret) {
		t.Fatal("wrong prefix should fail")
	}
}
