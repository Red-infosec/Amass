// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package datasrcs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OWASP/Amass/v3/config"
	"github.com/OWASP/Amass/v3/eventbus"
	"github.com/OWASP/Amass/v3/net/http"
	"github.com/OWASP/Amass/v3/requests"
	"github.com/OWASP/Amass/v3/systems"
)

// Shodan is the Service that handles access to the Shodan data source.
type Shodan struct {
	requests.BaseService

	API        *config.APIKey
	SourceType string
	sys        systems.System
}

// NewShodan returns he object initialized, but not yet started.
func NewShodan(sys systems.System) *Shodan {
	s := &Shodan{
		SourceType: requests.API,
		sys:        sys,
	}

	s.BaseService = *requests.NewBaseService(s, "Shodan")
	return s
}

// Type implements the Service interface.
func (s *Shodan) Type() string {
	return s.SourceType
}

// OnStart implements the Service interface.
func (s *Shodan) OnStart() error {
	s.BaseService.OnStart()

	s.API = s.sys.Config().GetAPIKey(s.String())
	if s.API == nil || s.API.Key == "" {
		s.sys.Config().Log.Printf("%s: API key data was not provided", s.String())
	}

	s.SetRateLimit(time.Second)
	return nil
}

// OnDNSRequest implements the Service interface.
func (s *Shodan) OnDNSRequest(ctx context.Context, req *requests.DNSRequest) {
	cfg := ctx.Value(requests.ContextConfig).(*config.Config)
	bus := ctx.Value(requests.ContextEventBus).(*eventbus.EventBus)
	if cfg == nil || bus == nil {
		return
	}

	re := cfg.DomainRegex(req.Domain)
	if re == nil || s.API == nil || s.API.Key == "" {
		return
	}

	s.CheckRateLimit()
	bus.Publish(requests.SetActiveTopic, eventbus.PriorityCritical, s.String())
	bus.Publish(requests.LogTopic, eventbus.PriorityHigh,
		fmt.Sprintf("Querying %s for %s subdomains", s.String(), req.Domain))

	url := s.restURL(req.Domain)
	headers := map[string]string{"Content-Type": "application/json"}
	page, err := http.RequestWebPage(url, nil, headers, "", "")
	if err != nil {
		bus.Publish(requests.LogTopic, eventbus.PriorityHigh, fmt.Sprintf("%s: %s: %v", s.String(), url, err))
		return
	}
	// Extract the subdomain names from the REST API results
	var m struct {
		Subdomains []string `json:"subdomains"`
	}
	if err := json.Unmarshal([]byte(page), &m); err != nil || len(m.Subdomains) == 0 {
		return
	}

	for _, sub := range m.Subdomains {
		name := sub + "." + req.Domain

		if re.MatchString(name) {
			bus.Publish(requests.NewNameTopic, eventbus.PriorityHigh, &requests.DNSRequest{
				Name:   name,
				Domain: req.Domain,
				Tag:    s.SourceType,
				Source: s.String(),
			})
		}
	}
}

func (s *Shodan) restURL(domain string) string {
	return fmt.Sprintf("https://api.shodan.io/dns/domain/%s?key=%s", domain, s.API.Key)
}
