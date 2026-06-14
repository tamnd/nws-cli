package nws

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions
// (Classify and Locate), which need no network. The client's HTTP behaviour
// and the full operation flow are covered in nws_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "nws" {
		t.Errorf("Scheme = %q, want nws", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != "api.weather.gov" {
		t.Errorf("Hosts = %v, want [api.weather.gov]", info.Hosts)
	}
	if info.Identity.Binary != "nws" {
		t.Errorf("Identity.Binary = %q, want nws", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"39.7456,-97.0892", "latlon", "39.7456,-97.0892"},
		{"TX", "state", "TX"},
		{"CA", "state", "CA"},
		{"some query", "query", "some query"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) returned error: %v", tc.in, err)
			continue
		}
		if typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)",
				tc.in, typ, id, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	cfg := DefaultConfig()

	cases := []struct {
		typ  string
		id   string
		want string
	}{
		{"latlon", "39.7456,-97.0892", cfg.BaseURL + "/points/39.7456,-97.0892"},
		{"state", "TX", cfg.BaseURL + "/alerts/active?area=TX"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil {
			t.Errorf("Locate(%q, %q) error: %v", tc.typ, tc.id, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Locate(%q, %q) = %q, want %q", tc.typ, tc.id, got, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("bogus", "x")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}
