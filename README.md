# k8s-cert-generator

This runs as a Kubernetes service that can provision TLS certificates from Let's
Encrypt and store them as Kubernetes secrets.

The certs are written to two different secrets:

```
--secret (default acme.secret): The acme/autocert internal cache. Don't modify this.

--ingress-secret (default acme.ingress.secret): Keys with names `tls.key` and
`tls.crt` will be written with the private and public key respectively.
```

The keys in the latter `ingress-secret` can be used by
Kubernetes to terminate TLS on the ingress, as described here:
https://kubernetes.io/docs/concepts/services-networking/ingress/#tls

## Usage

```
  -domain string
    	The domain to use
  -email string
    	The email registering the cert
  -ingress-secret string
    	Secret to use for storing ingress certificate (default "acme.ingress.secret")
  -namespace string
    	Namespace to use for cert storage.
  -port int
    	The port to listen on (default 8443)
  -secret string
    	Secret to use for cert storage (default "acme.secret")
  -staging
    	Use the letsencrypt staging server (default true)
```

### Ingress routing instructions

The ingress needs to route requests to the path `/.well-known` to your
k8s-cert-generator Kubernetes service.

```yaml
  - path: '/.well-known/*'
    backend:
      serviceName: k8s-cert-generator
      servicePort: 8443
```

### How it works

The heavy lifting is done by `acme/autocert` which provisions TLS certificates
for a domain automatically from Let's Encrypt. We provide a custom `Cache` for
storing those certificates, and in addition to the cache used by `autocert`
&mdash; the `secret` above &mdash; we write to a key that can be used by the
ingress to terminate TLS.

`autocert` contains logic to check when TLS certificates are about to expire and
renew them, so it should be sufficient to just keep the project running - you
don't have to periodically make requests to it or anything.

### Bootstrapping

You need to make a TLS request to trigger the Let's Encrypt logic, but if you
don't have TLS set up you won't be able to send TLS requests to your cluster
using an existing (good) certificate.

Set up HTTP port forwarding to the HTTP port (8442) in Kubernetes, so Let's
Encrypt can send requests over HTTP. Then enable port forwarding to the TLS
port, locally, so you can send a TLS request without having TLS set up on the
ingress.

```
kubectl port-forward k8s-cert-generator-55954596d7-gd8wd 8443:8443
```

Open a new Terminal shell and curl at localhost, being sure to set the domain
properly for SNI. You MUST enable certificate checking.

```
curl -vvv -i https://YOURDOMAIN.com:8443/.well-known/any-value --resolve YOURDOMAIN.com:8443:127.0.0.1
```

That should trigger the TLS logic to provision a certificate appropriately.

### Inspiration

Some code was borrowed (with heavy modification) from
github.com/micahhausler/k8s-acme-cache. That code is MIT licensed - the license
is also present at the top of k8cache.go.

The golang.org/x/crypto/acme/autocert code was written by the Go Authors. Please
see the LICENSE files in the vendor directory to view license ownership for the
various vendored libraries.
