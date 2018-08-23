// Code modified from https://github.com/micahhausler/k8s-acme-cache
//
// Copyright (c) 2017 Micah Hausler
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"context"
	"log"
	"strings"

	"golang.org/x/crypto/acme/autocert"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type kubernetesCache struct {
	Namespace string
	// Secret name used by Autocert for storing the raw cert data.
	SecretName        string
	IngressSecretName string
	Client            kubernetes.Interface
	deleteGracePeriod int64
}

// KubernetesCache returns an autocert.Cache that will store the certificate as
// a secret in Kubernetes. It accepts a secret name, namespace,
// kubernetes.Clientset, and grace period (in seconds)
func newKubernetesCache(secret, ingressSecret, namespace string, client kubernetes.Interface, deleteGracePeriod int64) autocert.Cache {
	return kubernetesCache{
		Namespace:         namespace,
		SecretName:        secret,
		IngressSecretName: ingressSecret,
		Client:            client,
		deleteGracePeriod: deleteGracePeriod,
	}
}

func (k kubernetesCache) Get(ctx context.Context, name string) ([]byte, error) {
	done := make(chan struct{})
	var err error
	var data []byte
	name = strings.Replace(name, "+", "-__plus__-", -1)

	go func() {
		var secret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName, meta_v1.GetOptions{})
		defer close(done)
		if err != nil {
			return
		}
		var ok bool
		data, ok = secret.Data[name]
		if !ok {
			err = autocert.ErrCacheMiss
			return
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("get %s: context error %v", name, ctx.Err())
		return nil, ctx.Err()
	case <-done:
	}
	if err != nil || len(data) == 0 {
		log.Printf("get %s: cache miss, returning error", name)
		return nil, autocert.ErrCacheMiss
	}
	log.Printf("get %s: data %s, err %v", name, string(data), err)
	return data, err
}

func isPrivateCert(keyName string) bool {
	return strings.HasSuffix(keyName, "-token")
}

func (k kubernetesCache) Put(ctx context.Context, name string, data []byte) error {
	name = strings.Replace(name, "+", "-__plus__-", -1)
	log.Printf("put %s: data %s", name, string(data))
	done := make(chan struct{})
	// data is something like this:
	//
	// -----BEGIN EC PRIVATE KEY-----
	// MHcCAQEEIItX06vEUTxdxHol3TK7UY5iZbmV5IqMl8LkqZ+MzmYcoAoGCCqGSM49
	// AwEHoUQDQgAEMbo7AGIkWBuC2wn+i5DIwEqH0/ZDi60sJkP/WPb5wh/KjGGPVFPi
	// nCknxPznJza/bjKWFMjIY9nYifz5vjItEA==
	// -----END EC PRIVATE KEY-----
	// -----BEGIN CERTIFICATE-----
	// MIIBKTCB0KADAgECAgEBMAoGCCqGSM49BAMCMAAwHhcNMTgwODIxMjIxMDM1WhcN
	// MTgxMTIwMDAxMDM1WjAAMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEMbo7AGIk
	// WBuC2wn+i5DIwEqH0/ZDi60sJkP/WPb5wh/KjGGPVFPinCknxPznJza/bjKWFMjI
	// Y9nYifz5vjItEKM7MDkwDgYDVR0PAQH/BAQDAgUgMAwGA1UdEwEB/wQCMAAwGQYD
	// VR0RAQH/BA8wDYILZXhhbXBsZS5vcmcwCgYIKoZIzj0EAwIDSAAwRQIgSLdJ5Fyv
	// 0dNBfirvo4mW9IFSL+ivN13/owI2f4FJdrMCIQDCS3DTd6UteYivWUz6RICWLN8y
	// kiBCmhurhHzzXp6OQQ==
	// -----END CERTIFICATE-----
	//
	// Reverse engineered the format by reading the acme/autocert docs. We need
	// to parse this out into public and private pieces so the Ingress can read
	// it in the format expected/declared by the Ingress docs, for example
	// here.
	//
	// https://github.com/kubernetes/ingress-gce/blob/master/README.md#secret
	var pub, priv []byte
	var err error
	if isPrivateCert(name) {
		pub, priv, err = getPrivPubBytes(data)
		if err != nil {
			log.Printf("put %s: returning err %v", name, err)
			return err
		}
	}
	go func() {
		defer close(done)

		var secret, ingressSecret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName, meta_v1.GetOptions{})
		if err != nil {
			return
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[name] = data
		select {
		case <-ctx.Done():
			return
		default:
			_, err = k.Client.CoreV1().Secrets(k.Namespace).Update(secret)
			if err == nil && isPrivateCert(name) {
				ingressSecret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.IngressSecretName, meta_v1.GetOptions{})
				if err != nil {
					return
				}
				if ingressSecret.Data == nil {
					ingressSecret.Data = make(map[string][]byte)
				}
				ingressSecret.Data["tls.crt"] = pub
				ingressSecret.Data["tls.key"] = priv
				_, err = k.Client.CoreV1().Secrets(k.Namespace).Update(ingressSecret)
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	log.Printf("put %s: return err %v", name, err)
	return err
}

func (k kubernetesCache) Delete(ctx context.Context, name string) error {
	name = strings.Replace(name, "+", "-__plus__-", -1)
	log.Printf("delete %s", name)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)

		var secret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName, meta_v1.GetOptions{})
		if err != nil {
			return
		}
		delete(secret.Data, name)

		select {
		case <-ctx.Done():
		default:
			orphanDependents := false
			// Don't overwrite the secret if the context was canceled.
			err = k.Client.CoreV1().Secrets(k.Namespace).Delete(k.SecretName, &meta_v1.DeleteOptions{
				GracePeriodSeconds: &k.deleteGracePeriod,
				OrphanDependents:   &orphanDependents,
			})
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	log.Printf("delete %s: return err %v", name, err)
	return err
}
