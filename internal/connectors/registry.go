package connectors

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// userAgent identifies this project to the ATSs it polls, per the same
// "identify yourself honestly" rule docs/design.md §5 applies to the
// scraper. A contact point matters more than politeness — it's how an ATS
// operator reaches us instead of just blocking the IP.
const userAgent = "job-syllabus/0.1 (+https://github.com/MayTheSForceBeWithYou/job-syllabus)"

// NewDefaultHTTPClient returns the shared client connectors should use.
// Client.Timeout bounds the whole round trip (dial + TLS + headers + body),
// but that alone hung for 34+ minutes in practice — worth not trusting on
// its own. The Transport below adds explicit sub-timeouts on each phase so
// a stuck DNS lookup or a server that accepts the connection but never
// sends headers fails fast and identifiably, instead of however Client.Timeout
// happens to interact with a hung phase.
func NewDefaultHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
}

// NewRegistry builds the ATS-name -> Connector map used to dispatch
// companies.yaml entries. Adding a company is a data change; adding an ATS
// is this function plus one new file.
func NewRegistry(client *http.Client) map[string]Connector {
	return map[string]Connector{
		"greenhouse":      NewGreenhouseConnector(client),
		"lever":           NewLeverConnector(client),
		"ashby":           NewAshbyConnector(client),
		"smartrecruiters": NewSmartRecruitersConnector(client),
		"workable":        NewWorkableConnector(client),
		"workday":         NewWorkdayConnector(client),
	}
}

// Get looks up a connector by ATS name from a registry built by NewRegistry.
func Get(registry map[string]Connector, ats string) (Connector, error) {
	c, ok := registry[ats]
	if !ok {
		return nil, fmt.Errorf("no connector registered for ats %q", ats)
	}
	return c, nil
}
