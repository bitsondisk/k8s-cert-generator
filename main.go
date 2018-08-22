package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func getBoolEnv(varname string) bool {
	result := os.Getenv(varname)
	switch result {
	case "false", "FALSE", "False", "0":
		return false
	default:
		return true
	}
}

var domain = flag.String("domain", "", "The domain to use")
var email = flag.String("email", "", "The email registering the cert")
var port = flag.Int("port", 8443, "The port to listen on")

var staging = flag.Bool("staging", getBoolEnv("STAGING"), "Use the letsencrypt staging server")

var namespace = flag.String("namespace", "", "Namespace to use for cert storage.")
var secretName = flag.String("secret", "acme.secret", "Secret to use for cert storage")
var ingressSecretName = flag.String("ingress-secret", "acme.ingress.secret", "Secret to use for storing ingress certificate")

func createInClusterClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func getNamespace() string {
	if len(*namespace) > 0 {
		return *namespace
	}
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}
	return "default"
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func main() {
	flag.Parse()
	client, err := createInClusterClient()
	if err != nil {
		log.Fatal(err)
	}

	cache := newKubernetesCache(*secretName, *ingressSecretName, getNamespace(), client, 1)
	var acmeClient *acme.Client
	if *staging {
		acmeClient = &acme.Client{DirectoryURL: "https://acme-staging.api.letsencrypt.org/directory"}
	}

	log.Printf("Creating cert manager for domain %s", *domain)
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(*domain),
		Cache:      cache,
		Email:      *email,
		Client:     acmeClient,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world"))
		log.Printf("Got request to %s", r.URL.String())
	})

	portString := fmt.Sprintf(":%d", *port)

	server := &http.Server{
		Addr:      portString,
		TLSConfig: certManager.TLSConfig(),
	}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Printf("Started TLS server on %s", server.Addr)
	// key and cert are coming from Let's Encrypt
	log.Fatal(server.ServeTLS(tcpKeepAliveListener{ln.(*net.TCPListener)}, "", ""))
}
