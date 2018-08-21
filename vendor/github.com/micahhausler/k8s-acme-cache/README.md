[![Build Status](https://travis-ci.org/micahhausler/k8s-acme-cache.svg)](https://travis-ci.org/micahhausler/k8s-acme-cache)
[![https://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square)](http://godoc.org/github.com/micahhausler/k8s-acme-cache/)

# k8s-acme-cache

An ACME [autocert](https://godoc.org/golang.org/x/crypto/acme/autocert#Cache)
cache that stores keys as Kubernetes secrets.

See the example application for a full example, but the basic usage looks like this
```go
import (
    "github.com/micahhausler/k8s-acme-cache"
    "golang.org/x/crypto/acme/autocert"
    "k8s.io/client-go/kubernetes"
)

cache := k8s_acme_cache.KubernetesCache(
    "my-acme-secret.secret",  // Secret Name
    "default",                // Namespace
    client,                   // Kubernetes client-go *kubernetes.ClientSet
    0,                        // Deletion Grace Period in seconds
)

certManager := autocert.Manager{
    Prompt:     autocert.AcceptTOS,
    HostPolicy: autocert.HostWhitelist("example.com"), //your domain here
    Cache:      cache,
}
```

## Required RBAC permissions

```yaml
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: Role
metadata:
  name: <role-name>
rules:
- apiGroups:
  - ""
  resources:
  - secret
  resourceNames:
  - <secret-name>
  verbs:
  - get
  - create
  - update
```

You'll also need a `RoleBinding` to bind the above role to the `ServiceAccount`
the application is assigned.

If the secret you want to use is in a different namespace than the application,
use a `ClusterRole`, and a `ClusterRoleBinding`

## License
MIT License. See [License](/LICENSE) for full text
