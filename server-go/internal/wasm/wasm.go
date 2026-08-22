// Package wasm is the P4.3 plugin runtime: WASM modules (via wazero, pure Go,
// air-gap safe) executing custom connector logic with strict limits — a memory
// cap and a per-invocation timeout. The JSON-in/JSON-out ABI (docs/decisions.md D2):
//
//	Guest MUST export:  memory, alloc(size i32) -> i32, execute(ptr i32, len i32) -> i32
//	Host provides "ff": result(ptr, len)      — publish the JSON result
//	                    log(ptr, len)          — emit a log line
//	                    http_request(ptr, len) -> i32 — egress-gated HTTP (status or -1)
//	                    http_response_len()    -> i32 — length of the last response body
//	                    response(ptr, len)     — copy the last response body into the guest
//
// The guest receives {"params":…,"input":…} on its input buffer. Secrets are
// never passed into the sandbox; ff.http_request is the only network path and
// routes through the policy module (safe-mode + egress allow-list).
package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/flowforge/flowforge/internal/policy"
)

// Limits cap plugin execution. Defaults: 512 pages (32 MiB), 5s.
type Limits struct {
	MemoryPages uint32 // in 64 KiB pages
	Timeout     time.Duration
}

// DefaultLimits are applied by Run.
var DefaultLimits = Limits{MemoryPages: 512, Timeout: 5 * time.Second}

const maxResponseBytes = 64 * 1024

// hostState carries the per-invocation state shared with host functions.
type hostState struct {
	pol      *policy.Policy
	client   *http.Client
	result   []byte
	logs     []string
	lastResp []byte
}

// Run executes a plugin once with the default limits.
func Run(module, input []byte, pol *policy.Policy) (string, []string, error) {
	return RunWithLimits(module, input, pol, DefaultLimits)
}

// RunWithLimits executes a plugin once with explicit limits.
func RunWithLimits(module, input []byte, pol *policy.Policy, lim Limits) (string, []string, error) {
	if lim.MemoryPages == 0 {
		lim.MemoryPages = DefaultLimits.MemoryPages
	}
	if lim.Timeout <= 0 {
		lim.Timeout = DefaultLimits.Timeout
	}
	if pol == nil {
		pol = &policy.Policy{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), lim.Timeout)
	defer cancel()

	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true). // interrupts execution on timeout
		WithMemoryLimitPages(lim.MemoryPages))
	defer r.Close(context.Background())

	st := &hostState{pol: pol, client: &http.Client{Timeout: lim.Timeout}}

	if err := buildHostModule(r, st); err != nil {
		return "", nil, err
	}

	mod, err := r.Instantiate(ctx, module)
	if err != nil {
		return "", nil, fmt.Errorf("instantiate: %w", err)
	}

	alloc := mod.ExportedFunction("alloc")
	exec := mod.ExportedFunction("execute")
	mem := mod.ExportedMemory("memory")
	if alloc == nil || exec == nil || mem == nil {
		return "", nil, fmt.Errorf("plugin must export alloc, execute, and memory")
	}

	// Pass the input buffer: ptr = alloc(len); write; execute(ptr, len).
	var execArgs [2]uint64
	if len(input) > 0 {
		ret, err := alloc.Call(ctx, uint64(len(input)))
		if err != nil || len(ret) != 1 {
			return "", nil, fmt.Errorf("alloc failed: %v", err)
		}
		ptr := uint32(ret[0])
		if uint64(ptr)+uint64(len(input)) > uint64(mem.Size()) {
			return "", nil, fmt.Errorf("plugin alloc returned out-of-range pointer %d", ptr)
		}
		if !mem.Write(ptr, input) {
			return "", nil, fmt.Errorf("write to plugin memory failed")
		}
		execArgs[0], execArgs[1] = uint64(ptr), uint64(len(input))
	}
	ret, err := exec.Call(ctx, execArgs[0], execArgs[1])
	if err != nil {
		return "", st.logs, execErr(err)
	}
	if len(ret) != 1 || ret[0] != 0 {
		code := -1
		if len(ret) == 1 {
			code = int(int32(ret[0]))
		}
		return "", st.logs, fmt.Errorf("plugin returned failure code %d", code)
	}
	return string(st.result), st.logs, nil
}

// execErr unwraps wazero traps into readable errors (timeout, OOM, …).
func execErr(err error) error {
	var se *sys.ExitError
	if errors.As(err, &se) {
		// Context cancellation / timeout surfaces as an exit error.
		return fmt.Errorf("plugin trapped (context done or non-zero exit): %w", err)
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		return fmt.Errorf("plugin exceeded its execution timeout")
	}
	return fmt.Errorf("plugin execution failed: %w", err)
}

// buildHostModule builds the "ff" host module bound to the invocation state.
func buildHostModule(r wazero.Runtime, st *hostState) error {
	b := r.NewHostModuleBuilder("ff")

	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, size uint32) {
			if buf, ok := m.Memory().Read(ptr, size); ok {
				st.result = append([]byte(nil), buf...)
			}
		}).
		Export("result")

	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, size uint32) {
			if buf, ok := m.Memory().Read(ptr, size); ok {
				st.logs = append(st.logs, string(buf))
			}
		}).
		Export("log")

	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, size uint32) uint32 {
			buf, ok := m.Memory().Read(ptr, size)
			if !ok {
				return 0xFFFFFFFF // -1: bad request buffer
			}
			return st.httpRequest(buf)
		}).
		Export("http_request")

	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context) uint32 {
			return uint32(len(st.lastResp))
		}).
		Export("http_response_len")

	b.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, ptr, size uint32) {
			n := uint32(len(st.lastResp))
			if n > size {
				n = size
			}
			if n > 0 {
				m.Memory().Write(ptr, st.lastResp[:n])
			}
		}).
		Export("response")

	_, err := b.Instantiate(context.Background())
	return err
}

// httpRequest performs the egress-gated call. Returns the status code, or
// 0xFFFFFFFF (-1) when blocked/failed — the guest sees a negative status.
func (st *hostState) httpRequest(buf []byte) uint32 {
	var req struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(buf, &req); err != nil {
		return 0xFFFFFFFF
	}
	if st.pol.SafeMode {
		st.logs = append(st.logs, "http blocked: safe-mode")
		return 0xFFFFFFFF
	}
	if !st.pol.EgressAllowed(req.URL) {
		st.logs = append(st.logs, "http blocked by egress policy: "+redact(req.URL))
		return 0xFFFFFFFF
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}
	hreq, err := http.NewRequest(method, req.URL, strings.NewReader(req.Body))
	if err != nil {
		return 0xFFFFFFFF
	}
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
	resp, err := st.client.Do(hreq)
	if err != nil {
		return 0xFFFFFFFF
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	st.lastResp = body
	return uint32(resp.StatusCode)
}

func redact(url string) string {
	if i := strings.IndexByte(url, '?'); i >= 0 {
		return url[:i] + "?…"
	}
	return url
}
