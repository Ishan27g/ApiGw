package pkg

import (
	"net"
	"net/http"
	"time"

	"github.com/Ishan27g/go-utils/tracing"
)

var defaultTransport http.RoundTripper = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

type service struct {
	upstream Upstream
	pxy      *RTPxy
	provider tracing.TraceProvider
}

func newService(upstream Upstream) service {
	return service{
		provider: tracing.Init("jaeger", "api-gw", upstream.Name),
		upstream: upstream,
		pxy:      newProxy(upstream.Addr, upstream.UrlPrefix),
	}
}
