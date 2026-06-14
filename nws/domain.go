package nws

import (
	"context"
	"regexp"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the US National Weather Service as a kit Domain: a driver
// that a multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/nws-cli/nws"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// nws:// URIs by routing to the operations Register installs. The same Domain
// also builds the standalone nws binary (see cli.NewApp), so the binary and a
// host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the NWS driver. It carries no state; the per-run client is built by
// the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "nws",
		Hosts:  []string{"api.weather.gov"},
		Identity: kit.Identity{
			Binary: "nws",
			Short:  "Read US National Weather Service data",
			Long: `nws reads public US National Weather Service (NWS) data over plain HTTPS,
shapes it into clean records, and prints output that pipes into the rest of your
tools. No API key needed.`,
			Site: "api.weather.gov",
			Repo: "https://github.com/tamnd/nws-cli",
		},
	}
}

// Register installs the client factory and every NWS operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// forecast: 7-day or hourly forecast for a lat/lon.
	kit.Handle(app, kit.OpMeta{Name: "forecast", Group: "weather", List: true,
		Summary: "7-day (or hourly) forecast for a location"}, doForecast)

	// alerts: active weather alerts for a US state.
	kit.Handle(app, kit.OpMeta{Name: "alerts", Group: "weather", List: true,
		Summary: "Active weather alerts for a US state"}, doAlerts)

	// stations: weather observation stations by state.
	kit.Handle(app, kit.OpMeta{Name: "stations", Group: "weather", List: true,
		Summary: "List weather observation stations by state"}, doStations)
}

// newClient builds the NWS client from the kit-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type forecastIn struct {
	Lat    float64 `kit:"flag" help:"latitude"`
	Lon    float64 `kit:"flag" help:"longitude"`
	Hourly bool    `kit:"flag" help:"return hourly periods instead of 7-day"`
	Client *Client `kit:"inject"`
}

type alertsIn struct {
	State  string  `kit:"flag" help:"2-letter US state code (e.g. TX)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type stationsIn struct {
	State  string  `kit:"flag" help:"2-letter US state code (e.g. TX)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func doForecast(ctx context.Context, in forecastIn, emit func(ForecastPeriod) error) error {
	periods, err := in.Client.Forecast(ctx, in.Lat, in.Lon, in.Hourly)
	if err != nil {
		return err
	}
	for _, p := range periods {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func doAlerts(ctx context.Context, in alertsIn, emit func(Alert) error) error {
	if in.State == "" {
		return errs.Usage("--state is required (e.g. --state TX)")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	alerts, err := in.Client.Alerts(ctx, in.State, limit)
	if err != nil {
		return err
	}
	for _, a := range alerts {
		if err := emit(a); err != nil {
			return err
		}
	}
	return nil
}

func doStations(ctx context.Context, in stationsIn, emit func(Station) error) error {
	if in.State == "" {
		return errs.Usage("--state is required (e.g. --state TX)")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	stations, err := in.Client.Stations(ctx, in.State, limit)
	if err != nil {
		return err
	}
	for _, s := range stations {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

var latlonRE = regexp.MustCompile(`^\d+\.\d+,-?\d+\.\d+$`)
var stateRE = regexp.MustCompile(`^[A-Z]{2}$`)

// Classify turns an input into (type, id) for ant URI routing.
func (Domain) Classify(input string) (uriType, id string, err error) {
	switch {
	case latlonRE.MatchString(input):
		return "latlon", input, nil
	case stateRE.MatchString(input):
		return "state", input, nil
	default:
		return "query", input, nil
	}
}

// Locate returns the live API URL for a (type, id) pair.
func (Domain) Locate(uriType, id string) (string, error) {
	cfg := DefaultConfig()
	switch uriType {
	case "latlon":
		return cfg.BaseURL + "/points/" + id, nil
	case "state":
		return cfg.BaseURL + "/alerts/active?area=" + id, nil
	default:
		return "", errs.Usage("nws has no resource type %q", uriType)
	}
}
