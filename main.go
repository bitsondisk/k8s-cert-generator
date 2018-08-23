package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kevinburke/handlers"
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
var httpPort = flag.Int("http-port", 8442, "The HTTP port to listen on")
var tlsPort = flag.Int("tls-port", 8443, "The TLS port to listen on")

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

// Graceful server shutdown.
func shutdownServer(server *http.Server) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	server.Shutdown(shutdownCtx)
	shutdownCancel()
}

func main() {
	flag.Parse()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(context.Background())

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

	tlsMux := http.NewServeMux()
	tlsMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world"))
		log.Printf("Got request to %s", r.URL.String())
	})
	tlsPortString := fmt.Sprintf(":%d", *tlsPort)
	tlsLogger := handlers.Logger.New("protocol", "https")
	server := &http.Server{
		Addr:      tlsPortString,
		Handler:   handlers.WithLogger(tlsMux, tlsLogger),
		TLSConfig: certManager.TLSConfig(),
	}
	go func() {
		ln, err := net.Listen("tcp", server.Addr)
		if err != nil {
			log.Fatal(err)
		}
		defer ln.Close()
		log.Printf("Started TLS server on %s", server.Addr)
		// key and cert are coming from Let's Encrypt
		serveErr := server.ServeTLS(tcpKeepAliveListener{ln.(*net.TCPListener)}, "", "")
		if serveErr != http.ErrServerClosed {
			log.Printf("Error starting TLS server: %v", serveErr)
			cancel()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello world"))
		log.Printf("Fallback handler called over HTTP: %s %s", r.Method, r.URL.String())
	})
	httpHandler := certManager.HTTPHandler(mux)
	httpPortString := fmt.Sprintf(":%d", *httpPort)
	httpLogger := handlers.Logger.New("protocol", "http")
	httpServer := &http.Server{
		Addr:    httpPortString,
		Handler: handlers.WithLogger(httpHandler, httpLogger),
	}
	go func() {
		ln, err := net.Listen("tcp", httpServer.Addr)
		if err != nil {
			log.Fatal(err)
		}
		defer ln.Close()
		log.Printf("Started HTTP server on %s", httpServer.Addr)
		serveErr := httpServer.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
		if serveErr != http.ErrServerClosed {
			log.Printf("Error starting http server: %v", serveErr)
			cancel()
		}
	}()

	select {
	case sig := <-c:
		fmt.Fprintf(os.Stderr, "Caught signal %v, shutting down...\n", sig)
	case <-ctx.Done():
	}

	cancel()
	// We could shut down each server concurrently but it's simple enough to do
	// consecutively and there's enough concurrency in this program.
	shutdownServer(server)
	shutdownServer(httpServer)
}
