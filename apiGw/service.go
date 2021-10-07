package apiGw

import (
	"log"
	"net/http/httputil"
	"net/url"
)

type service struct {
	upstream upstream
	proxy    *httputil.ReverseProxy
}

func newService(upstream upstream) service {
	proxy, err := newProxy(upstream.Addr)
	if err != nil {
		log.Fatal("bad host name")
	}
	return service{
		upstream: upstream,
		proxy:    proxy,
	}
}
func newProxy(dst string) (*httputil.ReverseProxy, error) {
	if url, err := url.Parse(dst); err == nil {
		proxy := httputil.NewSingleHostReverseProxy(url)
		return proxy, err
	} else {
		return nil, err
	}
}
