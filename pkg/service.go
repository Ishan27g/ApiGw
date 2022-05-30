package pkg

import (
	"net"
	"net/http"
	"time"
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
	upstream upstream
	pxy      *RTPxy
}

func newService(upstream upstream) service {
	return service{
		upstream: upstream,
		pxy:      newProxy(upstream.Addr, upstream.UrlPrefix),
	}
}
