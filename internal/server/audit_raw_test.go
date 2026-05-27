package server

import (
	"net/http/httptest"
	"testing"
)

// TestQueryTrue pins the truthy forms queryTrue accepts. The load-bearing case
// is "true": Radar Cloud's Hub fleet fan-out requests /api/audit?raw=true to
// skip local audit settings (the Hub owns effective Checks config). A silent
// drift here would re-introduce the settings-inversion the cloud unwind fixed.
func TestQueryTrue(t *testing.T) {
	cases := map[string]bool{
		"true":  true,
		"True":  true,
		"1":     true,
		"t":     true,
		"yes":   true,
		"false": false,
		"0":     false,
		"":      false,
		"raw":   false,
	}
	for val, want := range cases {
		r := httptest.NewRequest("GET", "/api/audit?raw="+val, nil)
		if got := queryTrue(r, "raw"); got != want {
			t.Errorf("queryTrue(raw=%q) = %v, want %v", val, got, want)
		}
	}
	// Absent param reads false.
	r := httptest.NewRequest("GET", "/api/audit", nil)
	if queryTrue(r, "raw") {
		t.Error("queryTrue with absent param = true, want false")
	}
}
