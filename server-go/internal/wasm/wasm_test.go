package wasm

// PLG-01..05: the WASM plugin runtime (P4.3). Modules are hand-crafted wasm
// binaries (a tiny section builder below) so the suite runs without any wasm
// toolchain — mirroring how plugins ship: bare .wasm files, no runtime deps.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowforge/flowforge/internal/policy"
)

// ---- tiny wasm binary builder ------------------------------------------------

const i32 = 0x7f

func uleb(n uint32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}

func wname(s string) []byte { return append(uleb(uint32(len(s))), s...) }

func vec(items ...[]byte) []byte {
	out := uleb(uint32(len(items)))
	for _, i := range items {
		out = append(out, i...)
	}
	return out
}

func section(id byte, content []byte) []byte {
	return append([]byte{id}, append(uleb(uint32(len(content))), content...)...)
}

// functype builds a type entry, e.g. functype(2, 0) == (i32,i32)->().
func functype(params, results int) []byte {
	out := []byte{0x60}
	out = append(out, uleb(uint32(params))...)
	for i := 0; i < params; i++ {
		out = append(out, i32)
	}
	out = append(out, uleb(uint32(results))...)
	for i := 0; i < results; i++ {
		out = append(out, i32)
	}
	return out
}

const (
	opI32Const = 0x41
	opCall     = 0x10
	opDrop     = 0x1a
	opLoop     = 0x03
	opBr       = 0x0c
	opEnd      = 0x0b
)

type modBuilder struct {
	types   [][]byte
	imports [][]byte // {module, field, typeIdx}
	funcs   []uint32 // type indices of defined funcs
	memMin  uint32
	exports [][]byte // {name, kind, idx}
	bodies  [][]byte // code bodies incl. final end
	data    []byte   // active data segment at offset 0
}

func (b *modBuilder) importFn(mod, field string, typeIdx uint32) {
	b.imports = append(b.imports, append(append(wname(mod), wname(field)...), append([]byte{0x00}, uleb(typeIdx)...)...))
}

func (b *modBuilder) exportFn(name string, idx uint32) {
	b.exports = append(b.exports, append(append(wname(name), 0x00), uleb(idx)...))
}

func (b *modBuilder) exportMem() {
	b.exports = append(b.exports, append(append(wname("memory"), 0x02), uleb(0)...))
}

func (b *modBuilder) body(code []byte) { b.bodies = append(b.bodies, code) }

func (b *modBuilder) build() []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	if len(b.types) > 0 {
		out = append(out, section(1, vec(b.types...))...)
	}
	if len(b.imports) > 0 {
		out = append(out, section(2, vec(b.imports...))...)
	}
	if len(b.funcs) > 0 {
		items := make([][]byte, len(b.funcs))
		for i, t := range b.funcs {
			items[i] = uleb(t)
		}
		out = append(out, section(3, vec(items...))...)
	}
	if b.memMin > 0 {
		out = append(out, section(5, append(uleb(1), append([]byte{0x00}, uleb(b.memMin)...)...))...)
	}
	if len(b.exports) > 0 {
		out = append(out, section(7, vec(b.exports...))...)
	}
	if len(b.bodies) > 0 {
		entries := make([][]byte, len(b.bodies))
		for i, body := range b.bodies {
			entry := append([]byte{0x00}, body...) // zero local groups
			entries[i] = append(uleb(uint32(len(entry))), entry...)
		}
		out = append(out, section(10, vec(entries...))...)
	}
	if len(b.data) > 0 {
		seg := append([]byte{0x00, opI32Const, 0x00, opEnd}, append(uleb(uint32(len(b.data))), b.data...)...)
		out = append(out, section(11, vec(seg))...)
	}
	return out
}

// modResult: publishes the fixed data segment via ff.result and returns 0.
func modResult(data string) []byte {
	b := &modBuilder{
		types:  [][]byte{functype(2, 0), functype(1, 1), functype(2, 1)},
		funcs:  []uint32{1, 2},
		memMin: 1,
		data:   []byte(data),
	}
	b.importFn("ff", "result", 0)
	b.exportMem()
	b.exportFn("alloc", 1) // import ff.result occupies func index 0
	b.exportFn("execute", 2)
	// alloc(n) -> 256
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	// execute(p, n) -> ff.result(0, len(data)); return 0
	b.body([]byte{opI32Const, 0x00, opI32Const, byte(len(data)), opCall, 0x00, opI32Const, 0x00, opEnd})
	return b.build()
}

// modLoop: execute spins forever (loop br 0) — timeout enforcement.
func modLoop() []byte {
	b := &modBuilder{types: [][]byte{functype(1, 1), functype(2, 1)}, funcs: []uint32{0, 1}, memMin: 1}
	b.exportMem()
	b.exportFn("alloc", 0)
	b.exportFn("execute", 1)
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	b.body([]byte{opLoop, 0x40, opBr, 0x00, opEnd, opI32Const, 0x00, opEnd})
	return b.build()
}

// modMemBig declares a 3-page memory — exceeding a 2-page limit.
func modMemBig() []byte {
	b := &modBuilder{types: [][]byte{functype(1, 1), functype(2, 1)}, funcs: []uint32{0, 1}, memMin: 3}
	b.exportMem()
	b.exportFn("alloc", 0)
	b.exportFn("execute", 1)
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	b.body([]byte{opI32Const, 0x00, opEnd})
	return b.build()
}

// modHTTP: calls ff.http_request with a JSON request {"method","url"} at
// offset 0 and RETURNS the status as the exit code (-1 blocked/fail ->
// plugin failure; guests that want success must drop the status and return 0
// — see dropStatus).
func modHTTP(url string, dropStatus bool) []byte {
	req := `{"method":"GET","url":"` + url + `"}`
	b := &modBuilder{
		types:  [][]byte{functype(2, 1), functype(1, 1), functype(2, 1)},
		funcs:  []uint32{1, 2},
		memMin: 1,
		data:   []byte(req),
	}
	b.importFn("ff", "http_request", 0)
	b.exportMem()
	b.exportFn("alloc", 1)
	b.exportFn("execute", 2)
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	body := []byte{opI32Const, 0x00, opI32Const, byte(len(req)), opCall, 0x00}
	if dropStatus {
		body = append(body, opDrop, opI32Const, 0x00)
	}
	b.body(append(body, opEnd))
	return b.build()
}

// ---- scenarios (PLG-01..05, see docs/test-strategy.md) ----------------------

func TestPLG01_SuccessWithResult(t *testing.T) {
	res, logs, err := Run(modResult(`{"ok":true}`), []byte(`{"x":1}`), &policy.Policy{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res != `{"ok":true}` {
		t.Errorf("result = %q", res)
	}
	if len(logs) != 0 {
		t.Errorf("logs = %v", logs)
	}
}

func TestPLG02_MemoryLimitEnforced(t *testing.T) {
	_, _, err := RunWithLimits(modMemBig(), nil, &policy.Policy{}, Limits{MemoryPages: 2, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "instantiate") {
		t.Fatalf("expected instantiation failure, got %v", err)
	}
}

func TestPLG03_TimeoutEnforced(t *testing.T) {
	start := time.Now()
	_, _, err := RunWithLimits(modLoop(), nil, &policy.Policy{}, Limits{MemoryPages: 512, Timeout: 150 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout took %v — execution was not interrupted", elapsed)
	}
}

func TestPLG04_HTTPEgressBlocked(t *testing.T) {
	_, logs, err := Run(modHTTP("http://blocked.example/x", false), nil,
		&policy.Policy{Allow: []string{"api.openai.com"}, DenyByDefault: true})
	if err == nil || !strings.Contains(err.Error(), "failure code -1") {
		t.Fatalf("expected failure code -1, got %v", err)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "blocked by egress policy") {
		t.Errorf("logs = %v, want egress-block note", logs)
	}
}

func TestPLG04b_HTTPAllowedReachesServer(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	host := hostOf(srv.URL)
	_, _, err := Run(modHTTP(srv.URL, true), nil, &policy.Policy{Allow: []string{host}, DenyByDefault: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("server hits = %d, want 1", hits)
	}
}

func TestPLG05_SafeModeBlocksHTTP(t *testing.T) {
	_, logs, err := Run(modHTTP("http://any.example/x", false), nil, &policy.Policy{SafeMode: true})
	if err == nil || !strings.Contains(err.Error(), "failure code -1") {
		t.Fatalf("expected failure code -1, got %v", err)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "safe-mode") {
		t.Errorf("logs = %v, want safe-mode note", logs)
	}
}

func hostOf(raw string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}
