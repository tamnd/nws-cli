package nws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/nws-cli/nws"
)

// pointsResp is a minimal /points response matching the real NWS API shape.
const pointsResp = `{
  "properties": {
    "gridId": "TOP",
    "gridX": 32,
    "gridY": 81,
    "timeZone": "America/Chicago",
    "forecast": "https://api.weather.gov/gridpoints/TOP/32,81/forecast",
    "relativeLocation": {
      "properties": {
        "city": "Montrose",
        "state": "KS"
      }
    }
  }
}`

// forecastResp is a minimal /gridpoints/.../forecast response.
const forecastResp = `{
  "properties": {
    "periods": [
      {
        "name": "This Afternoon",
        "shortForecast": "Sunny",
        "temperature": 73,
        "temperatureUnit": "F",
        "windSpeed": "5 mph",
        "windDirection": "SW",
        "isDaytime": true,
        "startTime": "2026-06-14T13:00:00-05:00"
      }
    ]
  }
}`

// alertsResp is a minimal /alerts/active response with the fields the spec requires.
const alertsResp = `{
  "features": [
    {
      "properties": {
        "id": "urn:oid:2.49.0.1.840.0.TEST",
        "event": "Rip Current Statement",
        "severity": "Moderate",
        "areaDesc": "Coastal Waters",
        "headline": "Rip Current Statement issued for Coastal Waters",
        "onset": "2026-06-14T12:00:00",
        "expires": "2026-06-15T06:00:00"
      }
    }
  ]
}`

// stationsResp is a minimal /stations response.
const stationsResp = `{
  "features": [
    {
      "properties": {
        "stationIdentifier": "KDFW",
        "name": "Dallas/Fort Worth Airport",
        "timeZone": "America/Chicago"
      }
    }
  ]
}`

// newRouteServer returns a test server that routes by URL path prefix.
// It always checks that User-Agent is present (NWS blocks requests without it).
func newRouteServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("request to %s carried no User-Agent", r.URL.Path)
			http.Error(w, "no user-agent", http.StatusForbidden)
			return
		}
		for prefix, body := range routes {
			plen := len(prefix)
			if len(r.URL.Path) >= plen && r.URL.Path[:plen] == prefix {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
}

func TestPoints(t *testing.T) {
	srv := newRouteServer(t, map[string]string{
		"/points/": pointsResp,
	})
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	gp, err := c.Points(context.Background(), "39.7456", "-97.0892")
	if err != nil {
		t.Fatalf("Points: %v", err)
	}
	if gp.GridID != "TOP" {
		t.Errorf("GridID = %q, want TOP", gp.GridID)
	}
	if gp.GridX != 32 {
		t.Errorf("GridX = %d, want 32", gp.GridX)
	}
	if gp.GridY != 81 {
		t.Errorf("GridY = %d, want 81", gp.GridY)
	}
	if gp.City != "Montrose" {
		t.Errorf("City = %q, want Montrose", gp.City)
	}
	if gp.State != "KS" {
		t.Errorf("State = %q, want KS", gp.State)
	}
	if gp.ForecastURL == "" {
		t.Error("ForecastURL should not be empty")
	}
}

func TestForecast(t *testing.T) {
	srv := newRouteServer(t, map[string]string{
		"/points/":     pointsResp,
		"/gridpoints/": forecastResp,
	})
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	periods, err := c.Forecast(context.Background(), "39.7456", "-97.0892", false)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if len(periods) != 1 {
		t.Fatalf("got %d periods, want 1", len(periods))
	}
	p := periods[0]
	if p.Name != "This Afternoon" {
		t.Errorf("Name = %q, want %q", p.Name, "This Afternoon")
	}
	if p.Temperature != 73 {
		t.Errorf("Temperature = %d, want 73", p.Temperature)
	}
	if p.TempUnit != "F" {
		t.Errorf("TempUnit = %q, want F", p.TempUnit)
	}
	if p.ShortForecast != "Sunny" {
		t.Errorf("ShortForecast = %q, want Sunny", p.ShortForecast)
	}
	if p.WindSpeed != "5 mph" {
		t.Errorf("WindSpeed = %q, want 5 mph", p.WindSpeed)
	}
}

func TestForecastHourly(t *testing.T) {
	var hourlyHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("request to %s carried no User-Agent", r.URL.Path)
		}
		if r.URL.Path == "/gridpoints/TOP/32,81/forecast/hourly" {
			hourlyHit = true
		}
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Path) >= len("/points/") && r.URL.Path[:len("/points/")] == "/points/" {
			_, _ = w.Write([]byte(pointsResp))
		} else {
			_, _ = w.Write([]byte(forecastResp))
		}
	}))
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	_, err := c.Forecast(context.Background(), "39.7456", "-97.0892", true)
	if err != nil {
		t.Fatalf("Forecast hourly: %v", err)
	}
	if !hourlyHit {
		t.Error("expected /forecast/hourly path to be hit")
	}
}

func TestAlerts(t *testing.T) {
	srv := newRouteServer(t, map[string]string{
		"/alerts/active": alertsResp,
	})
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	alerts, err := c.Alerts(context.Background(), "TX", 20)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(alerts))
	}
	a := alerts[0]
	if a.ID != "urn:oid:2.49.0.1.840.0.TEST" {
		t.Errorf("ID = %q", a.ID)
	}
	if a.Event != "Rip Current Statement" {
		t.Errorf("Event = %q, want %q", a.Event, "Rip Current Statement")
	}
	if a.Severity != "Moderate" {
		t.Errorf("Severity = %q, want Moderate", a.Severity)
	}
	if a.Area != "Coastal Waters" {
		t.Errorf("Area = %q, want Coastal Waters", a.Area)
	}
	if a.Headline == "" {
		t.Error("Headline should not be empty")
	}
	if a.Onset == "" {
		t.Error("Onset should not be empty")
	}
	if a.Expires == "" {
		t.Error("Expires should not be empty")
	}
}

func TestAlertsLimit(t *testing.T) {
	feats := make([]map[string]any, 5)
	for i := range feats {
		feats[i] = map[string]any{
			"properties": map[string]any{
				"id":       "urn:oid:test." + string(rune('0'+i)),
				"event":    "Test",
				"severity": "Minor",
			},
		}
	}
	body, _ := json.Marshal(map[string]any{"features": feats})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	alerts, err := c.Alerts(context.Background(), "TX", 2)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Errorf("got %d alerts, want 2 (limit must be respected)", len(alerts))
	}
}

func TestStations(t *testing.T) {
	srv := newRouteServer(t, map[string]string{
		"/stations": stationsResp,
	})
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	stations, err := c.Stations(context.Background(), "TX", 20)
	if err != nil {
		t.Fatalf("Stations: %v", err)
	}
	if len(stations) != 1 {
		t.Fatalf("got %d stations, want 1", len(stations))
	}
	s := stations[0]
	if s.ID != "KDFW" {
		t.Errorf("ID = %q, want KDFW", s.ID)
	}
	if s.Name != "Dallas/Fort Worth Airport" {
		t.Errorf("Name = %q", s.Name)
	}
	if s.State != "TX" {
		t.Errorf("State = %q, want TX", s.State)
	}
	if s.TimeZone != "America/Chicago" {
		t.Errorf("TimeZone = %q, want America/Chicago", s.TimeZone)
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(alertsResp))
	}))
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := nws.NewClient(cfg)

	start := time.Now()
	_, err := c.Alerts(context.Background(), "TX", 5)
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off (expected >= 500ms)")
	}
}

func TestUserAgentAlwaysPresent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(alertsResp))
	}))
	defer srv.Close()

	cfg := nws.DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	c := nws.NewClient(cfg)

	_, _ = c.Alerts(context.Background(), "TX", 5)
	if gotUA == "" {
		t.Error("no User-Agent header sent; NWS blocks requests without one")
	}
}
