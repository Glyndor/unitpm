// sign.go signs a file with an ed25519 private key and writes the raw
// 64-byte signature to <file>.sig. The private key is read from the
// RELEASE_SIGNING_KEY env var (base64-encoded 64-byte ed25519 seed+pub).
//
// Usage: go run scripts/sign.go <file>
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/sign.go <file>")
		os.Exit(1)
	}
	filePath := os.Args[1]

	keyB64 := os.Getenv("RELEASE_SIGNING_KEY")
	if keyB64 == "" {
		fmt.Fprintln(os.Stderr, "RELEASE_SIGNING_KEY not set")
		os.Exit(1)
	}
	keyRaw, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode key: %v\n", err)
		os.Exit(1)
	}
	if len(keyRaw) != ed25519.PrivateKeySize {
		fmt.Fprintf(os.Stderr, "key size %d, want %d\n", len(keyRaw), ed25519.PrivateKeySize)
		os.Exit(1)
	}
	privKey := ed25519.PrivateKey(keyRaw)

	body, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", filePath, err)
		os.Exit(1)
	}

	sig := ed25519.Sign(privKey, body)

	sigPath := filePath + ".sig"
	if err := os.WriteFile(sigPath, sig, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", sigPath, err)
		os.Exit(1)
	}
	fmt.Printf("Signed %s → %s (%d bytes)\n", filePath, sigPath, len(sig))
}
