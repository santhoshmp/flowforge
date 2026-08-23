package wasm

// PLG-06..09: deeper host-surface coverage — logging, HTTP response readback,
// module validation, hostile alloc. Feature F-EXT (P4.3).

import (
	"strings"
	"testing"

	"github.com/flowforge/flowforge/internal/policy"
)

// modLogResult: ff.log(data) then ff.result(data) — same buffer for both.
func modLogResult(data string) []byte {
	b := &modBuilder{
		types:  [][]byte{functype(2, 0), functype(1, 1), functype(2, 1)},
		funcs:  []uint32{1, 2},
		memMin: 1,
		data:   []byte(data),
	}
	b.importFn("ff", "result", 0)
	b.importFn("ff", "log", 0)
	b.exportMem()
	b.exportFn("alloc", 2) // imports occupy 0 (result) and 1 (log)
	b.exportFn("execute", 3)
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	// log(0,len); result(0,len); return 0
	b.body([]byte{opI32Const, 0x00, opI32Const, byte(len(data)), opCall, 0x01,
		opI32Const, 0x00, opI32Const, byte(len(data)), opCall, 0x00,
		opI32Const, 0x00, opEnd})
	return b.build()
}

// PLG-06: ff.log captures log lines that surface with the result.
func TestPLG06_LogHostFunction(t *testing.T) {
	res, logs, err := Run(modLogResult("step checkpoint"), nil, &policy.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if res != "step checkpoint" {
		t.Fatalf("result = %q", res)
	}
	if len(logs) != 1 || logs[0] != "step checkpoint" {
		t.Fatalf("logs = %v", logs)
	}
}

// PLG-07: garbage bytes are rejected at instantiation with a clear error.
func TestPLG07_InvalidModuleRejected(t *testing.T) {
	for _, bad := range [][]byte{
		nil,
		[]byte("not wasm at all"),
		{0x00, 0x61, 0x73, 0x6d}, // magic without version
	} {
		_, _, err := Run(bad, nil, &policy.Policy{})
		if err == nil {
			t.Fatalf("invalid module %v accepted", bad)
		}
	}
}

// PLG-08: a module missing the required exports is rejected by name.
func TestPLG08_MissingExportsRejected(t *testing.T) {
	b := &modBuilder{types: [][]byte{functype(0, 0)}, funcs: []uint32{0}, memMin: 1}
	b.exportMem()
	b.exportFn("execute", 0) // no alloc
	b.body([]byte{opEnd})
	_, _, err := Run(b.build(), nil, &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "alloc") {
		t.Fatalf("want missing-export error naming alloc, got %v", err)
	}
}

// PLG-09: alloc returning an out-of-range pointer is refused, not crashed.
func TestPLG09_HostileAllocPointer(t *testing.T) {
	b := &modBuilder{types: [][]byte{functype(1, 1), functype(2, 1)}, funcs: []uint32{0, 1}, memMin: 1}
	b.exportMem()
	b.exportFn("alloc", 0)
	b.exportFn("execute", 1)
	// alloc(n) -> 0x7FFFFFFF
	b.body([]byte{opI32Const, 0xFF, 0xFF, 0xFF, 0xFF, 0x07, opEnd})
	// execute(p,n) -> 0
	b.body([]byte{opI32Const, 0x00, opEnd})
	_, _, err := Run(b.build(), []byte(`{"x":1}`), &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "out-of-range") {
		t.Fatalf("want out-of-range alloc error, got %v", err)
	}
}

// PLG-10 (bonus): non-zero exit code surfaces as a failure with the code.
func TestPLG10_NonZeroExitCode(t *testing.T) {
	b := &modBuilder{types: [][]byte{functype(1, 1), functype(2, 1)}, funcs: []uint32{0, 1}, memMin: 1}
	b.exportMem()
	b.exportFn("alloc", 0)
	b.exportFn("execute", 1)
	b.body([]byte{opI32Const, 0x80, 0x02, opEnd})
	b.body([]byte{opI32Const, 0x07, opEnd}) // return 7
	_, _, err := Run(b.build(), nil, &policy.Policy{})
	if err == nil || !strings.Contains(err.Error(), "failure code 7") {
		t.Fatalf("want failure code 7, got %v", err)
	}
}
