package apiGw

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const (
	RequestMaxWaitTime = 5 * time.Second
)

type ApiGw interface {
	// Start api-gw, non-blocking
	Start(close chan bool)
	// RegisterService registers a host to redirect requests that match the urlPrefix
	registerService(upstream upstream, ctx context.Context, check bool)
	// GetServiceProxy returns the http.ReverseProxy for the host that matched the urlPrefix
	getServiceProxy(urlPrefix string) (string, *httputil.ReverseProxy)
}

func NewFromFile(fileName string) ApiGw {
	config := read(fileName)
	return NewFromConfig(config)
}

type proxy struct {
	client  http.Client
	addr    string
	delay   time.Duration
	proxies map[string]service // url-prefix <-> service
}

func (p *proxy) Start(close chan bool) {
	go p.start(close)
}

func NewFromConfig(config Config) ApiGw {
	ctx, cancel := context.WithTimeout(context.Background(), RequestMaxWaitTime)
	defer cancel()
	delay, _ := time.ParseDuration(config.Delay)
	proxy := &proxy{
		client: http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 3 * time.Second,
				}).DialContext,
			},
		},
		delay:   delay,
		proxies: map[string]service{},
		addr:    config.Listen,
	}

	for _, service := range config.Upstreams {
		proxy.registerService(*service, ctx, config.Check)
	}
	if len(proxy.proxies) == 0 {
		log.Fatalln("no upstreams detected")
	}
	for _, service := range proxy.proxies {
		log.Print(service.upstream.UrlPrefix, "\t-> \t", service.upstream.Addr)
	}
	return proxy
}

func (p *proxy) start(close chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		if pHost, pHandler := p.getServiceProxy(r.URL.Path); pHandler != nil {
			<-time.After(p.delay)
			logResponseStatus(pHandler, r.Method, r.URL, pHost, now)
			pHandler.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
	go func() {
		log.Print("Gateway Started on ", p.addr)
		log.Print(http.ListenAndServe(p.addr, nil))
	}()
	<-close
	log.Print("Gateway stopped")

}

// RegisterService adds a new api destination for the urlPrefix
func (p *proxy) registerService(upstream upstream, ctx context.Context, check bool) {
	if check {
		if checkHostConnection(p.client, upstream.Addr, ctx) {
			p.proxies[upstream.UrlPrefix] = newService(upstream)
		}
	} else {
		p.proxies[upstream.UrlPrefix] = newService(upstream)
	}
}

// GetServiceProxy matches the urlPrefix & returns corresponding reverseProxy
func (p *proxy) getServiceProxy(urlPrefix string) (string, *httputil.ReverseProxy) {
	for proxyUrl, service := range p.proxies {
		if strings.HasPrefix(urlPrefix, proxyUrl) {
			return service.upstream.Addr, service.proxy
		}
	}
	return "", nil
}

// logResponseStatus modifies the response handler to log the response status
func logResponseStatus(pHandler *httputil.ReverseProxy, method string, r *url.URL, pHost string, then time.Time) {
	pHandler.ModifyResponse = func(resp *http.Response) error {
		log.Print(time.Since(then).Microseconds(), "us ", "- [", method, "] ", r, "\t -> \t", pHost, " ", resp.StatusCode)
		return nil
	}
}
