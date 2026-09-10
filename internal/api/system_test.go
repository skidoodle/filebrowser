package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSystemSettings_Get(t *testing.T) {
	t.Parallel()
	ts, _, _ := newInsecureTestServer(t, 10<<20)
	defer ts.Close()

	res := do(t, newTestReq(t, http.MethodGet, ts.URL+"/api/settings/system", nil))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings/system: want 200, got %d", res.StatusCode)
	}
	var initial SystemSettingsResponse
	if err := json.NewDecoder(res.Body).Decode(&initial); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = res.Body.Close()

	if initial.Dynamic.MaxUpload <= 0 {
		t.Errorf("expected max_upload > 0, got %d", initial.Dynamic.MaxUpload)
	}
	if initial.Info.OS == "" || initial.Info.GoVersion == "" {
		t.Errorf("expected info.os and info.go_version to be populated, got %+v", initial.Info)
	}
}

func TestSystemSettings_Update(t *testing.T) {
	t.Parallel()
	ts, _, _ := newInsecureTestServer(t, 10<<20)
	defer ts.Close()

	newMaxUpload := int64(50 << 20)
	newMaxText := int64(5 << 20)
	newGuard := false
	newReqRate := 120
	newDlRate := int64(50 << 20)
	newPow := 2
	newProxies := "10.0.0.0/8, 192.168.1.1/32"

	updateBody := UpdateSystemSettingsRequest{
		MaxUpload:      &newMaxUpload,
		MaxTextSize:    &newMaxText,
		Guard:          &newGuard,
		RequestRate:    &newReqRate,
		DownloadRate:   &newDlRate,
		PowDifficulty:  &newPow,
		TrustedProxies: &newProxies,
	}
	b, err := json.Marshal(updateBody)
	if err != nil {
		t.Fatal(err)
	}

	putRes := do(t, newTestReq(t, http.MethodPut, ts.URL+"/api/settings/system", strings.NewReader(string(b))))
	if putRes.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/settings/system: want 200, got %d", putRes.StatusCode)
	}
	var updated SystemSettingsResponse
	if err := json.NewDecoder(putRes.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = putRes.Body.Close()

	if updated.Dynamic.MaxUpload != newMaxUpload {
		t.Errorf("max_upload: want %d, got %d", newMaxUpload, updated.Dynamic.MaxUpload)
	}
	if updated.Dynamic.Guard != newGuard {
		t.Errorf("guard: want %v, got %v", newGuard, updated.Dynamic.Guard)
	}

	getRes := do(t, newTestReq(t, http.MethodGet, ts.URL+"/api/settings/system", nil))
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/settings/system after update: want 200, got %d", getRes.StatusCode)
	}
	var reloaded SystemSettingsResponse
	if err := json.NewDecoder(getRes.Body).Decode(&reloaded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if reloaded.Dynamic.MaxUpload != newMaxUpload {
		t.Errorf("reloaded max_upload: want %d, got %d", newMaxUpload, reloaded.Dynamic.MaxUpload)
	}
}

func TestSystemSettings_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		json string
	}{
		{"negative max_upload", `{"max_upload": -1}`},
		{"zero request_rate", `{"request_rate": 0}`},
		{"negative pow_difficulty", `{"pow_difficulty": -1}`},
		{"too high pow_difficulty", `{"pow_difficulty": 17}`},
		{"invalid proxy CIDR", `{"trusted_proxies": "invalid-cidr"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts, _, _ := newInsecureTestServer(t, 10<<20)
			defer ts.Close()
			res := do(t, newTestReq(t, http.MethodPut, ts.URL+"/api/settings/system", strings.NewReader(tc.json)))
			defer res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("want 400 for %s, got %d", tc.name, res.StatusCode)
			}
		})
	}
}
