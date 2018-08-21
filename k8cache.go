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

	"golang.org/x/crypto/acme/autocert"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type kubernetesCache struct {
	Namespace         string
	SecretName        string
	Client            kubernetes.Interface
	deleteGracePeriod int64
}

// KubernetesCache returns an autocert.Cache that will store the certificate as
// a secret in Kubernetes. It accepts a secret name, namespace,
// kubernetes.Clientset, and grace period (in seconds)
func KubernetesCache(secret, namespace string, client kubernetes.Interface, deleteGracePeriod int64) autocert.Cache {
	return kubernetesCache{
		Namespace:         namespace,
		SecretName:        secret,
		Client:            client,
		deleteGracePeriod: deleteGracePeriod,
	}
}

func (k kubernetesCache) Get(ctx context.Context, name string) ([]byte, error) {
	done := make(chan struct{})
	var err error
	var data []byte

	go func() {
		var secret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName, meta_v1.GetOptions{})
		defer close(done)
		if err != nil {
			return
		}
		data = secret.Data[name]
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}
	if err != nil {
		return nil, autocert.ErrCacheMiss
	}
	return data, err
}

func (k kubernetesCache) Put(ctx context.Context, name string, data []byte) error {
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)

		var secret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName, meta_v1.GetOptions{})
		if err != nil {
			return
		}
		secret.Data[name] = data

		select {
		case <-ctx.Done():
		default:
			// Don't overwrite the secret if the context was canceled.
			_, err = k.Client.CoreV1().Secrets(k.Namespace).Update(secret)
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return err
}

func (k kubernetesCache) Delete(ctx context.Context, name string) error {
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
	return err
}
