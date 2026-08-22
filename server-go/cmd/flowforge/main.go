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
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/flowforge/flowforge/internal/api"
	"github.com/flowforge/flowforge/internal/connectors"
	"github.com/flowforge/flowforge/internal/engine"
	"github.com/flowforge/flowforge/internal/policy"
	"github.com/flowforge/flowforge/internal/signing"
	"github.com/flowforge/flowforge/internal/spec"
	"github.com/flowforge/flowforge/internal/store"
	"github.com/flowforge/flowforge/internal/wasm"
	"github.com/flowforge/flowforge/ui"
)

// version is overridden at release time via
// -ldflags "-X main.version=v1.2.3" (see scripts/build.* and the release workflow).
var version = "0.1.0-dev"

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
	case "connectors":
		listConnectorsCmd()
	case "connector":
		connectorCmd(os.Args[2:])
	case "plugin":
		pluginCmd(os.Args[2:])
	case "keygen":
		keygenCmd(flagSet(os.Args[2:]))
	case "sign":
		signCmd(flagSet(os.Args[2:]))
	case "verify":
		verifyCmd(flagSet(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

// ---- artifact signing (F-DSL-03) --------------------------------------------

// flagSet is a minimal -k/--key value flag parser: returns the key flag value
// plus the positional args.
func flagSet(args []string) (key string, rest []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-k", "--key":
			if i+1 >= len(args) {
				fail("--key requires a path")
			}
			i++
			key = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	return key, rest
}

func keygenCmd(key string, rest []string) {
	dir := "."
	if len(rest) > 0 && rest[0] != "" {
		dir = rest[0]
	}
	privPath, pubPath, err := signing.GenerateKeyFiles(dir)
	exitOnErr(err)
	fmt.Printf("private key: %s (keep secret — signs artifacts)\n", privPath)
	fmt.Printf("public key:  %s (share — verifies artifacts)\n", pubPath)
}

func signCmd(key string, rest []string) {
	if len(rest) != 1 {
		fail("usage: flowforge sign <file.flow.yaml> [--key flowforge.key]")
	}
	keyPath := key
	if keyPath == "" {
		keyPath = os.Getenv("FLOWFORGE_SIGNING_KEY")
	}
	if keyPath == "" {
		keyPath = "flowforge.key"
	}
	priv, err := signing.LoadPrivateKey(keyPath)
	exitOnErr(err)
	sig, err := signing.SignFile(rest[0], priv)
	exitOnErr(err)
	fmt.Printf("signed — %s\n", sig)
}

func verifyCmd(key string, rest []string) {
	if len(rest) != 1 {
		fail("usage: flowforge verify <file.flow.yaml> [--key flowforge.key.pub]")
	}
	keyPath := key
	if keyPath == "" {
		keyPath = "flowforge.key.pub"
	}
	pub, err := signing.LoadPublicKey(keyPath)
	exitOnErr(err)
	if err := signing.VerifyFile(rest[0], pub); err != nil {
		fail(err.Error())
	}
	fmt.Printf("verified — %s\n", rest[0])
}

func listConnectorsCmd() {
	reg, err := connectors.NewRegistry(connectors.DefaultUserDir())
	exitOnErr(err)
	for _, e := range reg.List() {
		src := "builtin"
		if !e.Builtin {
			src = e.Dir
		}
		fmt.Printf("%-18s %-14s %-8s %s\n", e.Manifest.ID, e.Manifest.Executor, e.Manifest.Version, src)
	}
	fmt.Println(registryCountNote(len(reg.List())))
}

func registryCountNote(n int) string {
	if n == 1 {
		return "1 connector loaded"
	}
	return fmt.Sprintf("%d connectors loaded (built-ins embedded; drop dirs into FLOWFORGE_CONNECTOR_DIR to add more)", n)
}

func connectorCmd(args []string) {
	if len(args) == 0 {
		fail("usage: flowforge connector <validate|test> <dir> [input.json]")
	}
	switch args[0] {
	case "validate":
		requireArg("connector validate", args, 1)
		e, err := connectors.ParseDir(args[1])
		exitOnErr(err)
		fmt.Printf("valid — %s v%s (executor: %s, auth: %s)\n", e.Manifest.ID, e.Manifest.Version, e.Manifest.Executor, e.Manifest.Auth.Mode)
	case "test":
		requireArg("connector test", args, 1)
		testConnectorDir(args[1], args[2:], false)
	default:
		fail("usage: flowforge connector <validate|test> <dir> [input.json]")
	}
}

func pluginCmd(args []string) {
	if len(args) == 0 || args[0] != "test" {
		fail("usage: flowforge plugin test <dir> [input.json]")
	}
	requireArg("plugin test", args, 1)
	testConnectorDir(args[1], args[2:], true)
}

// testConnectorDir dry-runs a connector directory: manifest + params validation
// and a redacted preview (wasm executors run their module sandboxed).
func testConnectorDir(dir string, rest []string, requireWASM bool) {
	e, err := connectors.ParseDir(dir)
	exitOnErr(err)
	if requireWASM && e.Manifest.Executor != connectors.KindWASM {
		fail("plugin test requires executor: wasm (got " + e.Manifest.Executor + ")")
	}
	params, input := loadTestInput(rest)
	if err := e.Manifest.ValidateParams(params); err != nil {
		fail("params: " + err.Error())
	}
	preview, warnings, err := connectors.Preview(e, params, input)
	exitOnErr(err)
	for k, v := range preview {
		fmt.Printf("  %-8s %v\n", k+":", v)
	}
	for _, w := range warnings {
		fmt.Println("  warning: " + w)
	}
	if e.Manifest.Executor == connectors.KindWASM {
		module := connectors.ModuleBytes(e)
		if len(module) == 0 {
			fail("wasm module not found: " + e.Manifest.WASM.Module)
		}
		inputJSON, _ := json.Marshal(map[string]any{"params": params, "input": input})
		res, logs, runErr := wasm.Run(module, inputJSON, policy.FromEnv(os.Getenv))
		if len(logs) > 0 {
			for _, l := range logs {
				fmt.Println("  log: " + l)
			}
		}
		exitOnErr(runErr)
		fmt.Printf("  result:  %s\n", res)
	}
	fmt.Println("test ok — no request was sent for http/smtp executors")
}

// loadTestInput reads optional params/input from a JSON file ({"params":…,"input":…}).
func loadTestInput(rest []string) (map[string]string, map[string]any) {
	params := map[string]string{}
	input := map[string]any{}
	if len(rest) > 0 && rest[0] != "" {
		raw, err := os.ReadFile(rest[0])
		exitOnErr(err)
		var doc struct {
			Params map[string]string `json:"params"`
			Input  map[string]any    `json:"input"`
		}
		exitOnErr(json.Unmarshal(raw, &doc))
		if doc.Params != nil {
			params = doc.Params
		}
		if doc.Input != nil {
			input = doc.Input
		}
	}
	return params, input
}

func requireArg(cmd string, args []string, n int) {
	if len(args) <= n {
		fail(fmt.Sprintf("usage: flowforge %s <dir>", cmd))
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
	fmt.Println("  connectors           list installed connectors (built-ins + drop-in dir)")
	fmt.Println("  connector validate <dir>            validate a connector directory")
	fmt.Println("  connector test <dir> [input.json]   dry-run a connector (preview; wasm runs sandboxed)")
	fmt.Println("  plugin test <dir> [input.json]      run a wasm plugin connector sandboxed")
	fmt.Println("  keygen [dir]                        generate an artifact signing keypair")
	fmt.Println("  sign <file> [--key <priv>]          sign a flowforge/v1 artifact (writes <file>.sig)")
	fmt.Println("  verify <file> [--key <pub>]         verify an artifact signature")
	fmt.Println()
	fmt.Println("env (serve): PORT, DB_PATH, OPENAI_API_KEY, OPENAI_BASE_URL, OPENAI_MODEL")
	fmt.Println("env (ext):   FLOWFORGE_CONNECTOR_DIR, FLOWFORGE_SECRETS_FILE, FLOWFORGE_SECRETS_KEY")
	fmt.Println("            FLOWFORGE_SAFE_MODE, FLOWFORGE_EGRESS_ALLOW")
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
