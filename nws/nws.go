// Package nws is the library behind the nws command line:
// the HTTP client, request shaping, and the typed data models for the US
// National Weather Service public API (api.weather.gov). No API key required.
package nws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds tunables for the NWS client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns a Config with conservative NWS-appropriate defaults.
// NWS blocks requests that carry no User-Agent.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.weather.gov",
		UserAgent: "nws-cli/0.1 (github.com/tamnd/nws-cli)",
		Rate:      100 * time.Millisecond,
		Retries:   3,
		Timeout:   15 * time.Second,
	}
}

// Client talks to api.weather.gov.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client with the provided config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// --- Output types ---

// ForecastPeriod is one period in a 7-day or hourly forecast.
type ForecastPeriod struct {
	Name             string `json:"name"`
	ShortForecast    string `json:"short_forecast"`
	Temperature      int    `json:"temperature"`
	TempUnit         string `json:"temp_unit"`
	WindSpeed        string `json:"wind_speed"`
	WindDirection    string `json:"wind_direction"`
	IsDaytime        bool   `json:"is_daytime"`
	StartTime        string `json:"start_time"`
	DetailedForecast string `json:"detailed_forecast"`
}

// Alert is one active weather alert.
type Alert struct {
	Event       string `json:"event"`
	Severity    string `json:"severity"`
	AreaDesc    string `json:"area_desc"`
	Effective   string `json:"effective"`
	Expires     string `json:"expires"`
	Description string `json:"description"`
}

// Station is a weather observation station.
type Station struct {
	ID       string `kit:"id" json:"id"`
	Name     string `json:"name"`
	TimeZone string `json:"timezone"`
	State    string `json:"state"`
}

// --- Wire types (NWS API shapes) ---

type wirePoints struct {
	Properties struct {
		CWA      string `json:"cwa"`
		GridX    int    `json:"gridX"`
		GridY    int    `json:"gridY"`
		TimeZone string `json:"timeZone"`
		City     string `json:"city"`
		State    string `json:"state"`
	} `json:"properties"`
}

type wireForecast struct {
	Properties struct {
		Periods []wirePeriod `json:"periods"`
	} `json:"properties"`
}

type wirePeriod struct {
	Name             string `json:"name"`
	ShortForecast    string `json:"shortForecast"`
	DetailedForecast string `json:"detailedForecast"`
	Temperature      int    `json:"temperature"`
	TemperatureUnit  string `json:"temperatureUnit"`
	WindSpeed        string `json:"windSpeed"`
	WindDirection    string `json:"windDirection"`
	IsDaytime        bool   `json:"isDaytime"`
	StartTime        string `json:"startTime"`
}

type wireAlerts struct {
	Features []struct {
		Properties wireAlert `json:"properties"`
	} `json:"features"`
}

type wireAlert struct {
	Event       string `json:"event"`
	Severity    string `json:"severity"`
	AreaDesc    string `json:"areaDesc"`
	Effective   string `json:"effective"`
	Expires     string `json:"expires"`
	Description string `json:"description"`
}

type wireStations struct {
	Features []struct {
		Properties wireStation `json:"properties"`
	} `json:"features"`
}

type wireStation struct {
	StationIdentifier string `json:"stationIdentifier"`
	Name              string `json:"name"`
	TimeZone          string `json:"timeZone"`
}

// --- Client methods ---

// Forecast returns 7-day (or hourly) forecast periods for a lat/lon.
// It first resolves the NWS grid via /points, then fetches /gridpoints/forecast.
func (c *Client) Forecast(ctx context.Context, lat, lon float64, hourly bool) ([]ForecastPeriod, error) {
	// Step 1: resolve lat/lon to office + grid coords.
	pointsURL := fmt.Sprintf("%s/points/%.4f,%.4f", c.cfg.BaseURL, lat, lon)
	body, err := c.get(ctx, pointsURL)
	if err != nil {
		return nil, fmt.Errorf("points lookup: %w", err)
	}
	var pts wirePoints
	if err := json.Unmarshal(body, &pts); err != nil {
		return nil, fmt.Errorf("decode points: %w", err)
	}
	p := pts.Properties
	if p.CWA == "" {
		return nil, fmt.Errorf("no grid office in response (lat=%.4f lon=%.4f)", lat, lon)
	}

	// Step 2: fetch forecast from the grid.
	fURL := fmt.Sprintf("%s/gridpoints/%s/%d,%d/forecast", c.cfg.BaseURL, p.CWA, p.GridX, p.GridY)
	if hourly {
		fURL += "/hourly"
	}
	body, err = c.get(ctx, fURL)
	if err != nil {
		return nil, fmt.Errorf("forecast fetch: %w", err)
	}
	var wf wireForecast
	if err := json.Unmarshal(body, &wf); err != nil {
		return nil, fmt.Errorf("decode forecast: %w", err)
	}

	out := make([]ForecastPeriod, 0, len(wf.Properties.Periods))
	for _, wp := range wf.Properties.Periods {
		out = append(out, ForecastPeriod{
			Name:             wp.Name,
			ShortForecast:    wp.ShortForecast,
			Temperature:      wp.Temperature,
			TempUnit:         wp.TemperatureUnit,
			WindSpeed:        wp.WindSpeed,
			WindDirection:    wp.WindDirection,
			IsDaytime:        wp.IsDaytime,
			StartTime:        wp.StartTime,
			DetailedForecast: wp.DetailedForecast,
		})
	}
	return out, nil
}

// Alerts returns active weather alerts for a 2-letter US state code.
func (c *Client) Alerts(ctx context.Context, state string, limit int) ([]Alert, error) {
	u := fmt.Sprintf("%s/alerts/active?area=%s", c.cfg.BaseURL, state)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("alerts fetch: %w", err)
	}
	var wa wireAlerts
	if err := json.Unmarshal(body, &wa); err != nil {
		return nil, fmt.Errorf("decode alerts: %w", err)
	}
	out := make([]Alert, 0, len(wa.Features))
	for i, f := range wa.Features {
		if limit > 0 && i >= limit {
			break
		}
		a := f.Properties
		out = append(out, Alert{
			Event:       a.Event,
			Severity:    a.Severity,
			AreaDesc:    a.AreaDesc,
			Effective:   a.Effective,
			Expires:     a.Expires,
			Description: a.Description,
		})
	}
	return out, nil
}

// Stations returns weather observation stations for a 2-letter US state code.
func (c *Client) Stations(ctx context.Context, state string, limit int) ([]Station, error) {
	u := fmt.Sprintf("%s/stations?state=%s&limit=%d", c.cfg.BaseURL, state, limit)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("stations fetch: %w", err)
	}
	var ws wireStations
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("decode stations: %w", err)
	}
	out := make([]Station, 0, len(ws.Features))
	for _, f := range ws.Features {
		s := f.Properties
		out = append(out, Station{
			ID:       s.StationIdentifier,
			Name:     s.Name,
			TimeZone: s.TimeZone,
			State:    state,
		})
	}
	return out, nil
}

// --- HTTP plumbing ---

// get fetches a URL with pacing, User-Agent, and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/geo+json, application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d from %s", resp.StatusCode, rawURL)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace enforces the minimum gap between requests.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
