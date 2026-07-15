// sign.go signs a file with an ed25519 private key and writes the raw
// 64-byte signature to <file>.sig. The private key is read from the
// RELEASE_SIGNING_KEY env var, base64 (std) encoded, as either a 32-byte
// seed or a 64-byte seed+pub.
//
// The org signing secret (GLYNDOR_RELEASE_ED25519_KEY) is a raw 32-byte
// seed -- that is what podup's signer reads, and what signed podup 1.9.1.
// Accepting only the 64-byte form is why this signer could never use the
// shared key, and why unitpm ended up on a release key of its own.
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
	var privKey ed25519.PrivateKey
	switch len(keyRaw) {
	case ed25519.SeedSize:
		privKey = ed25519.NewKeyFromSeed(keyRaw)
	case ed25519.PrivateKeySize:
		privKey = ed25519.PrivateKey(keyRaw)
	default:
		fmt.Fprintf(os.Stderr, "key size %d, want %d (seed) or %d (seed+pub)\n",
			len(keyRaw), ed25519.SeedSize, ed25519.PrivateKeySize)
		os.Exit(1)
	}

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
