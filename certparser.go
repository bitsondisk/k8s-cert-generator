package main

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// Attempt to parse the given private key DER block. OpenSSL 0.9.8 generates
// PKCS#1 private keys by default, while OpenSSL 1.0.0 generates PKCS#8 keys.
// OpenSSL ecparam generates SEC1 EC private keys for ECDSA. We try all three.
//
// Inspired by parsePrivateKey in crypto/tls/tls.go.
func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey:
			return key, nil
		case *ecdsa.PrivateKey:
			return key, nil
		default:
			return nil, errors.New("acme/autocert: unknown private key type in PKCS#8 wrapping")
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	return nil, errors.New("acme/autocert: failed to parse private key")
}

// certKey is the key by which certificates are tracked in state, renewal and cache.
type certKey struct {
	domain  string // without trailing dot
	isRSA   bool   // RSA cert for legacy clients (as opposed to default ECDSA)
	isToken bool   // tls-based challenge token cert; key type is undefined regardless of isRSA
}

// cacheGet always returns a valid certificate, or an error otherwise.
// If a cached certificate exists but is not valid, ErrCacheMiss is returned.
func getPrivPubBytes(data []byte) ([]byte, []byte, error) {
	// private
	priv, pub := pem.Decode(data)
	if priv == nil || !strings.Contains(priv.Type, "PRIVATE") {
		return nil, nil, errors.New("could not parse first part of data as private key")
	}
	_, err := parsePrivateKey(priv.Bytes)
	if err != nil {
		return nil, nil, err
	}
	buf := new(bytes.Buffer)
	if err := pem.Encode(buf, priv); err != nil {
		return nil, nil, err
	}
	pubCopy := make([]byte, len(pub))
	copy(pubCopy[:], pub)
	// public
	for len(pub) > 0 {
		var b *pem.Block
		b, pub = pem.Decode(pub)
		if b == nil {
			break
		}
	}
	if len(pub) > 0 {
		// Leftover content not consumed by pem.Decode. Corrupt. Ignore.
		return nil, nil, fmt.Errorf("leftover content not public or private key")
	}
	return buf.Bytes(), pubCopy, nil
}
