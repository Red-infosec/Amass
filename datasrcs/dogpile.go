// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package datasrcs

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/OWASP/Amass/v3/config"
	"github.com/OWASP/Amass/v3/eventbus"
	"github.com/OWASP/Amass/v3/net/http"
	"github.com/OWASP/Amass/v3/requests"
	"github.com/OWASP/Amass/v3/systems"
)

// Dogpile is the Service that handles access to the Dogpile data source.
type Dogpile struct {
	requests.BaseService

	SourceType string
	quantity   int
	limit      int
}

// NewDogpile returns he object initialized, but not yet started.
func NewDogpile(sys systems.System) *Dogpile {
	d := &Dogpile{
		SourceType: requests.SCRAPE,
		quantity:   15, // Dogpile returns roughly 15 results per page
		limit:      90,
	}

	d.BaseService = *requests.NewBaseService(d, "Dogpile")
	return d
}

// Type implements the Service interface.
func (d *Dogpile) Type() string {
	return d.SourceType
}

// OnStart implements the Service interface.
func (d *Dogpile) OnStart() error {
	d.BaseService.OnStart()

	d.SetRateLimit(time.Second)
	return nil
}

// OnDNSRequest implements the Service interface.
func (d *Dogpile) OnDNSRequest(ctx context.Context, req *requests.DNSRequest) {
	cfg := ctx.Value(requests.ContextConfig).(*config.Config)
	bus := ctx.Value(requests.ContextEventBus).(*eventbus.EventBus)
	if cfg == nil || bus == nil {
		return
	}

	re := cfg.DomainRegex(req.Domain)
	if re == nil {
		return
	}
	bus.Publish(requests.LogTopic, eventbus.PriorityHigh,
		fmt.Sprintf("Querying %s for %s subdomains", d.String(), req.Domain))

	num := d.limit / d.quantity
	for i := 0; i < num; i++ {
		select {
		case <-d.Quit():
			return
		default:
			d.CheckRateLimit()
			bus.Publish(requests.SetActiveTopic, eventbus.PriorityCritical, d.String())

			u := d.urlByPageNum(req.Domain, i)
			page, err := http.RequestWebPage(u, nil, nil, "", "")
			if err != nil {
				bus.Publish(requests.LogTopic, eventbus.PriorityHigh, fmt.Sprintf("%s: %s: %v", d.String(), u, err))
				return
			}

			for _, sd := range re.FindAllString(page, -1) {
				bus.Publish(requests.NewNameTopic, eventbus.PriorityHigh, &requests.DNSRequest{
					Name:   cleanName(sd),
					Domain: req.Domain,
					Tag:    d.SourceType,
					Source: d.String(),
				})
			}
		}
	}
}

func (d *Dogpile) urlByPageNum(domain string, page int) string {
	qsi := strconv.Itoa(d.quantity * page)
	u, _ := url.Parse("http://www.dogpile.com/search/web")

	u.RawQuery = url.Values{"qsi": {qsi}, "q": {domain}}.Encode()
	return u.String()
}
