package connectors

import (
	"fmt"
	"net/http"
	"time"
)

// userAgent identifies this project to the ATSs it polls, per the same
// "identify yourself honestly" rule docs/design.md §5 applies to the
// scraper. A contact point matters more than politeness — it's how an ATS
// operator reaches us instead of just blocking the IP.
const userAgent = "job-syllabus/0.1 (+https://github.com/MayTheSForceBeWithYou/job-syllabus)"

// NewDefaultHTTPClient returns the shared client connectors should use:
// bounded timeout, no surprises.
func NewDefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// NewRegistry builds the ATS-name -> Connector map used to dispatch
// companies.yaml entries. Adding a company is a data change; adding an ATS
// is this function plus one new file.
func NewRegistry(client *http.Client) map[string]Connector {
	return map[string]Connector{
		"greenhouse": NewGreenhouseConnector(client),
		"lever":      NewLeverConnector(client),
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
