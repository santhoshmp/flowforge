package api

// MERGE-01..05: MDM stewardship resolution (F-MDM-RESOLVE).

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// addPendingRecord creates a pending-stewardship vendor via the public API.
func addPendingRecord(t *testing.T, hs *httptest.Server, id string) {
	t.Helper()
	code, b := req(t, hs, "POST", "/api/v1/mdm/vendors", map[string]any{
		"record": map[string]string{"id": id, "name": "Test Vendor " + id, "country": "NL", "tax_id": "NL-" + id},
	})
	if code != 200 {
		t.Fatalf("add record: %d %s", code, b)
	}
}

func recordStatus(t *testing.T, hs *httptest.Server, id string) string {
	t.Helper()
	_, b := req(t, hs, "GET", "/api/v1/mdm/vendors", nil)
	for _, rec := range asMap(b)["records"].([]any) {
		r := rec.(map[string]any)
		if r["id"] == id {
			if _, ok := r["status"]; ok {
				return r["status"].(string)
			}
		}
	}
	return "" // not found
}

// MERGE-01: promote turns a pending record golden.
func TestMERGE01_Promote(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	addPendingRecord(t, hs, "V-9001")

	code, b := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "V-9001", "action": "promote"})
	if code != 200 {
		t.Fatalf("promote: %d %s", code, b)
	}
	if got := recordStatus(t, hs, "V-9001"); got != "golden" {
		t.Fatalf("status after promote = %q", got)
	}
	_, b = req(t, hs, "GET", "/api/v1/audit", nil)
	if !strings.Contains(string(b), "MDM record promoted") {
		t.Fatalf("promote not audited: %.200s", b)
	}
}

// MERGE-02: merge removes the pending record into an existing golden one.
func TestMERGE02_Merge(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	addPendingRecord(t, hs, "V-9002")

	// Find any golden vendor to merge into.
	_, b := req(t, hs, "GET", "/api/v1/mdm/vendors", nil)
	golden := ""
	for _, rec := range asMap(b)["records"].([]any) {
		r := rec.(map[string]any)
		if r["status"] == "golden" && golden == "" {
			golden = r["id"].(string)
		}
	}
	if golden == "" {
		t.Fatal("no golden vendor to merge into")
	}

	code, b := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "V-9002", "action": "merge", "mergeInto": golden})
	if code != 200 {
		t.Fatalf("merge: %d %s", code, b)
	}
	if got := recordStatus(t, hs, "V-9002"); got != "" {
		t.Fatalf("merged record still present (status %q)", got)
	}
	_, b = req(t, hs, "GET", "/api/v1/audit", nil)
	if !strings.Contains(string(b), "merged into "+golden) {
		t.Fatalf("merge not audited: %.200s", b)
	}
}

// MERGE-03: reject deletes the pending record with an audit entry.
func TestMERGE03_Reject(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()
	addPendingRecord(t, hs, "V-9003")

	code, _ := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "V-9003", "action": "reject"})
	if code != 200 {
		t.Fatalf("reject: %d", code)
	}
	if got := recordStatus(t, hs, "V-9003"); got != "" {
		t.Fatalf("rejected record still present (status %q)", got)
	}
}

// MERGE-04: golden records and unknown records cannot be resolved.
func TestMERGE04_Guards(t *testing.T) {
	hs, st := newTestServer(t)
	defer hs.Close()
	defer st.Close()

	// Unknown record.
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "nope", "action": "reject"}); code != 404 {
		t.Fatalf("unknown record: %d", code)
	}
	// Golden record refuses.
	_, b := req(t, hs, "GET", "/api/v1/mdm/vendors", nil)
	golden := ""
	for _, rec := range asMap(b)["records"].([]any) {
		r := rec.(map[string]any)
		if r["status"] == "golden" && golden == "" {
			golden = r["id"].(string)
		}
	}
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": golden, "action": "reject"}); code != 400 {
		t.Fatalf("golden record resolved: %d", code)
	}
	// Invalid action.
	addPendingRecord(t, hs, "V-9004")
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "V-9004", "action": "delete"}); code != 400 {
		t.Fatalf("invalid action: %d", code)
	}
	// Merge without a golden target.
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/vendors/resolve", map[string]string{"id": "V-9004", "action": "merge", "mergeInto": "nope"}); code != 400 {
		t.Fatalf("merge into unknown: %d", code)
	}
	// Unknown entity.
	if code, _ := req(t, hs, "POST", "/api/v1/mdm/nothing/resolve", map[string]string{"id": "x", "action": "reject"}); code != 404 {
		t.Fatalf("unknown entity: %d", code)
	}
}
