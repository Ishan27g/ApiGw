package pkg

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Ishan27g/go-utils/tracing"
	"github.com/jedib0t/go-pretty/v6/table"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

const (
	RequestMaxWaitTime = 5 * time.Second
)

type ApiGw interface {
	// Start api-pkg, non-blocking
	Start(close chan bool)
	// Add a service
	Add(u *Upstream)
	Remove(u *Upstream)
	// Handler() http.HandlerFunc
	// GetServiceProxy returns the http.ReverseProxy for the host that matched the urlPrefix
	getServiceProxy(urlPrefix string) *service
	printProxyTable()
}

func NewFromFile(fileName string) ApiGw {

	if config, _ := ReadConfFile(fileName); config.Listen != "" {
		return NewFromConfig(config)
	}

	return nil
}

type apiGw struct {
	sync.Mutex
	client  http.Client
	check   bool
	addr    string
	delay   time.Duration
	proxies map[string]upstreamGroup // url-prefix <-> upstreamGroup
	tp      tracing.TraceProvider
}

func (a *apiGw) Add(upstream *Upstream) {
	a.Lock()
	defer a.Unlock()
	if a.check { // ping before connection
		ctx, cancel := context.WithTimeout(context.Background(), RequestMaxWaitTime)
		defer cancel()
		if !checkHostConnection(a.client, upstream.Addr, ctx) {
			return
		}
	}
	if a.proxies[upstream.UrlPrefix] == nil {
		a.proxies[upstream.UrlPrefix] = asGroup(upstream)
		return
	}
	a.proxies[upstream.UrlPrefix].AddHost(upstream)
}

func asGroup(upstream *Upstream) upstreamGroup {
	return newUpsGroup(balance{
		Addr:      []string{upstream.Addr},
		names:     []string{upstream.Name},
		UrlPrefix: upstream.UrlPrefix,
	})
}

func (a *apiGw) Remove(upstream *Upstream) {
	a.Lock()
	defer a.Unlock()
	if a.proxies[upstream.UrlPrefix] == nil {
		log.Println("\t\tCannot remove - ", upstream.Addr)
		return
	}
	if a.proxies[upstream.UrlPrefix].RemoveHost(upstream.Addr) == 0 {
		delete(a.proxies, upstream.UrlPrefix)
	}
}

func (a *apiGw) Start(close chan bool) {
	a.start(close)
}

func NewFromConfig(config Config) ApiGw {

	delay, _ := time.ParseDuration(config.Delay)
	proxy := &apiGw{
		Mutex:   sync.Mutex{},
		client:  *otelhttp.DefaultClient,
		check:   config.Check,
		delay:   delay,
		addr:    config.Listen,
		proxies: make(map[string]upstreamGroup),
		tp:      tracing.Init("jaeger", "api-gw", ""),
	}

	// read balancer from config
	for _, group := range config.Balancer {
		for _, s := range group.Addr {
			proxy.Add(&Upstream{
				Name:      "",
				Addr:      s,
				UrlPrefix: group.UrlPrefix,
			})
		}
	}
	// read balancer from config
	for _, ups := range config.Upstreams {
		proxy.Add(&Upstream{
			Name:      ups.Name,
			Addr:      ups.Addr,
			UrlPrefix: ups.UrlPrefix,
		})
	}

	if len(proxy.proxies) == 0 {
		log.Println("no upstreams detected")
	}

	// proxy.printProxyTable()
	return proxy
}

func (a *apiGw) printProxyTable() {
	a.Lock()
	defer a.Unlock()

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.AppendHeader(table.Row{"Name", "Upstream Group", "Destination"})

	var forEachUpstreamGroup = func(cb func(upstreamGroup), upstreamGroups map[string]upstreamGroup) {
		for _, upStreamGroup := range upstreamGroups {
			cb(upStreamGroup)
		}
	}
	var forEachUpstreamHost = func(cb func(*service), upStreamGroup upstreamGroup) {
		for _, _ = range upStreamGroup.GetHosts() {
			service := upStreamGroup.GetNext()
			cb(service)
		}
	}

	forEachUpstreamGroup(func(upStreamGroup upstreamGroup) {
		var logs []table.Row
		forEachUpstreamHost(func(s *service) {
			logs = append(logs, table.Row{s.upstream.Name, upStreamGroup.GetUrlPrefix(), s.upstream.Addr})
		}, upStreamGroup)
		t.AppendRows(logs)
		t.AppendSeparator()
	}, a.proxies)
	t.Render()
}

func (a *apiGw) start(close chan bool) {

	provider := tracing.Init("jaeger", "api-gw", "proxy")
	defer provider.Close()

	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		service := a.getServiceProxy(r.URL.Path)
		if service == nil {
			http.NotFound(w, r)
			return
		}
		if name, pHost, pxy := service.upstream.Name, service.upstream.Addr, service.pxy; pxy != nil {
			ctx, span := service.provider.Get().Start(r.Context(), name+"|"+r.RequestURI)
			if span.IsRecording() {
				log.Println("span is recording", span.SpanContext().SpanID())
			}
			defer span.End()
			<-time.After(a.delay)
			span.SetAttributes(attribute.Key("proxy_balance_group").String(name))
			span.SetAttributes(attribute.Key("proxy_Delay").String(a.delay.String()))
			span.SetAttributes(attribute.Key("proxy_To").String(pHost))
			span.SetAttributes(attribute.Key("endpoint").String(r.URL.Path))
			rspStatus := pxy.RoundTrip(ctx, span, w, r)
			span.SetAttributes(attribute.Key("proxy_ResponseStatus").Int(rspStatus))

			log.Print(time.Since(now).Microseconds(), "us ", "- [", r.RequestURI, "] ", "\t -> \t", pHost, " ", rspStatus)
		} else {
			http.NotFound(w, r)
		}
	}), "proxy"))

	go func() {
		a.printProxyTable()
		log.Print("Gateway Started on ", a.addr)
		log.Print(http.ListenAndServe(a.addr, nil))
	}()
	<-close
	log.Print("Gateway stopped")

}

// GetServiceProxy matches the urlPrefix & returns corresponding reverseProxy
func (a *apiGw) getServiceProxy(urlPrefix string) *service {
	a.Lock()
	defer a.Unlock()
	for proxyUrl, upStreamGroup := range a.proxies {
		if strings.HasPrefix(urlPrefix, proxyUrl) {
			service := upStreamGroup.GetNext()
			return service
		}
	}
	return nil
}
