package k8s_acme_cache

import (
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/context"
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
// kubrenetes.Clientset, and grace period (in seconds)
func KubernetesCache(secret, namespace string, client kubernetes.Interface, deleteGracePeriod int64) autocert.Cache {
	return kubernetesCache{
		Namespace:         namespace,
		SecretName:        secret,
		Client:            client,
		deleteGracePeriod: deleteGracePeriod,
	}
}

func (k kubernetesCache) Get(ctx context.Context, name string) ([]byte, error) {
	var (
		data []byte
		done = make(chan struct{})
		err  error
	)

	go func() {
		var secret *v1.Secret
		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName)
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
	var (
		err  error
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		var secret *v1.Secret

		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName)
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
	var (
		err  error
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		var secret *v1.Secret

		secret, err = k.Client.CoreV1().Secrets(k.Namespace).Get(k.SecretName)
		if err != nil {
			return
		}
		delete(secret.Data, name)

		select {
		case <-ctx.Done():
		default:
			var (
				orphanDependents bool  = false
			)
			// Don't overwrite the secret if the context was canceled.
			err = k.Client.CoreV1().Secrets(k.Namespace).Delete(k.SecretName, &v1.DeleteOptions{
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
