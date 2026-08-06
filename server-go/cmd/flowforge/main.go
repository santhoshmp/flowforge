// Command flowforge is the distributable entrypoint.
//
//	subcommands: version | validate <file> | run <file> | serve
//
// `serve` runs the full control plane (HTTP API + durable engine + embedded UI)
// backed by SQLite. The other commands work with flowforge/v1 artifacts.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/flowforge/flowforge/internal/api"
	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/spec"
	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/ui"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("flowforge %s\n", version)
	case "validate":
		requireFile("validate")
		s, err := loadSpec(os.Args[2])
		exitOnErr(err)
		fmt.Printf("valid — %s\n", s.Summary())
	case "run":
		requireFile("run")
		s, err := loadSpec(os.Args[2])
		exitOnErr(err)
		fmt.Printf("plan: %s\n", s.Summary())
		for i, st := range s.Spec.Steps {
			fmt.Printf("  %d. [%s] %s\n", i+1, st.Type, st.Name)
		}
		fmt.Println("note: durable execution engine runs via `serve`; this is a plan preview.")
	case "serve":
		runServe()
	default:
		usage()
		os.Exit(2)
	}
}

func runServe() {
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "flowforge.db")
	authMode := envOr("FLOWFORGE_AUTH", "auto")
	pol := policy.FromEnv(os.Getenv)

	st, err := store.Open(dbPath)
	if err != nil {
		fail(err.Error())
	}
	if err := st.SeedIfEmpty(); err != nil {
		fail(err.Error())
	}

	srv := api.New(st, authMode)
	if fsys, err := ui.DistFS(); err == nil {
		srv.EnableUI(fsys)
	}
	go startScheduler(st, pol)

	scheme := "http"
	addr := ":" + port
	httpServer := &http.Server{Addr: addr, Handler: srv}
	fmt.Printf("FlowForge control plane on %s://localhost:%s (db: %s)\n", scheme, port, dbPath)
	fmt.Printf("  auth: %s  |  %s\n", authMode, pol.Summary())

	if stringsEqualFold(envOr("FLOWFORGE_TLS", "off"), "on") {
		cert, err := selfSignedCert()
		if err != nil {
			fail("tls: " + err.Error())
		}
		httpServer.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		scheme = "https"
		fmt.Printf("  tls: self-signed (browser will warn; trust it for local use)\n")
		if err := httpServer.ListenAndServeTLS("", ""); err != nil {
			fail(err.Error())
		}
		return
	}
	if err := httpServer.ListenAndServe(); err != nil {
		fail(err.Error())
	}
}

// selfSignedCert generates an ephemeral ECDSA self-signed certificate for localhost.
func selfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost", Organization: []string{"FlowForge"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}

func stringsEqualFold(a, b string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return len(a) == len(b)
}

func startScheduler(st *store.Store, pol *policy.Policy) {
	t := time.NewTicker(850 * time.Millisecond)
	for range t.C {
		engine.TickAll(st, pol)
	}
}

func requireFile(cmd string) {
	if len(os.Args) < 3 {
		fail(fmt.Sprintf("usage: flowforge %s <file.flow.yaml>", cmd))
	}
}

func loadSpec(file string) (*spec.WorkflowSpec, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return spec.ParseYAML(string(b))
}

func usage() {
	fmt.Println("usage: flowforge <command> [args]")
	fmt.Println("commands:")
	fmt.Println("  version              print the version")
	fmt.Println("  validate <file>      parse + validate a flowforge/v1 artifact")
	fmt.Println("  run <file>           parse, validate, and preview the execution plan")
	fmt.Println("  serve                run the control plane (API + engine + UI) on :8080")
	fmt.Println()
	fmt.Println("env (serve): PORT, DB_PATH, OPENAI_API_KEY, OPENAI_BASE_URL, OPENAI_MODEL")
}

func exitOnErr(err error) {
	if err != nil {
		fail(err.Error())
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
