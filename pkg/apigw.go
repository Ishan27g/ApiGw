package pkg

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	RequestMaxWaitTime = 5 * time.Second
)

type ApiGw interface {
	// Start api-pkg, non-blocking
	Start(close chan bool)
	// Add a service
	Add(u *upstream)
	Remove(u *upstream)
	// Handler() http.HandlerFunc
	// GetServiceProxy returns the http.ReverseProxy for the host that matched the urlPrefix
	getServiceProxy(urlPrefix string) (string, *RTPxy)
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
}

func (a *apiGw) Add(upstream *upstream) {
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

func asGroup(upstream *upstream) upstreamGroup {
	return newUpsGroup(balance{
		Addr:      []string{upstream.Addr},
		UrlPrefix: upstream.UrlPrefix,
	})
}

func (a *apiGw) Remove(upstream *upstream) {
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
		Mutex: sync.Mutex{},
		client: http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: RequestMaxWaitTime, // only for initial health-check
				}).DialContext,
			},
		},
		check:   config.Check,
		delay:   delay,
		addr:    config.Listen,
		proxies: make(map[string]upstreamGroup),
	}

	// read balancer from config
	for _, group := range config.Balancer {
		for _, s := range group.Addr {
			proxy.Add(&upstream{
				Name:      "",
				Addr:      s,
				UrlPrefix: group.UrlPrefix,
			})
		}
	}
	// read balancer from config
	for _, ups := range config.Upstreams {
		proxy.Add(&upstream{
			Name:      "",
			Addr:      ups.Addr,
			UrlPrefix: ups.UrlPrefix,
		})
	}

	if len(proxy.proxies) == 0 {
		log.Println("no upstreams detected")
	}

	proxy.printProxyTable()
	return proxy
}

func (a *apiGw) printProxyTable() {
	a.Lock()
	defer a.Unlock()

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.AppendHeader(table.Row{"Upstream Group", "Destination"})

	var forEachUpstreamGroup = func(cb func(upstreamGroup), upstreamGroups map[string]upstreamGroup) {
		for _, upStreamGroup := range upstreamGroups {
			cb(upStreamGroup)
		}
	}
	var forEachUpstreamHost = func(cb func(string), upStreamGroup upstreamGroup) {
		for _, upstream := range upStreamGroup.GetHosts() {
			cb(upstream)
		}
	}

	forEachUpstreamGroup(func(upStreamGroup upstreamGroup) {
		var logs []table.Row
		forEachUpstreamHost(func(host string) {
			logs = append(logs, table.Row{upStreamGroup.GetUrlPrefix(), host})
		}, upStreamGroup)
		t.AppendRows(logs)
		t.AppendSeparator()
	}, a.proxies)
	t.Render()
}

func (a *apiGw) start(close chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		if pHost, pxy := a.getServiceProxy(r.URL.Path); pxy != nil {
			<-time.After(a.delay)
			rspStatus := pxy.RoundTrip(w, r)
			log.Print(time.Since(now).Microseconds(), "us ", "- [", r.RequestURI, "] ", "\t -> \t", pHost, " ", rspStatus)
		} else {
			http.NotFound(w, r)
		}
	})
	go func() {
		log.Print("Gateway Started on ", a.addr)
		log.Print(http.ListenAndServe(a.addr, nil))
	}()
	<-close
	log.Print("Gateway stopped")

}

// GetServiceProxy matches the urlPrefix & returns corresponding reverseProxy
func (a *apiGw) getServiceProxy(urlPrefix string) (string, *RTPxy) {
	a.Lock()
	defer a.Unlock()
	for proxyUrl, upStreamGroup := range a.proxies {
		if strings.HasPrefix(urlPrefix, proxyUrl) {
			service := *upStreamGroup.GetNext()
			return service.upstream.Addr, service.pxy
		}
	}
	return "", nil
}
