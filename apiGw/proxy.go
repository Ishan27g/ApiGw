package apiGw

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	RequestMaxWaitTime = 5 * time.Second
)

type ApiGw interface {
	// Start api-apiGw, non-blocking
	Start(close chan bool)
	// RegisterService registers a host to redirect requests that match the urlPrefix
	setUpstreamGroup(upStreamGroup upstreamGroup, ctx context.Context, check bool)
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

type proxy struct {
	client  http.Client
	addr    string
	delay   time.Duration
	proxies map[string]upstreamGroup // url-prefix <-> upstreamGroup
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
		proxies: map[string]upstreamGroup{},
		addr:    config.Listen,
	}

	var upStreamGroups []upstreamGroup

	// read balancer from config
	for _, group := range config.Balancer {
		g := *group
		upStreamGroups = append(upStreamGroups, new(g))
	}

	// read upstream from config
	for _, group := range convertUpstream()(config.Upstreams) {
		upStreamGroups = append(upStreamGroups, new(group))
	}

	// save upstream group
	for _, upStreamGroup := range upStreamGroups {
		proxy.setUpstreamGroup(upStreamGroup, ctx, config.Check)
	}
	if len(proxy.proxies) == 0 {
		log.Fatalln("no upstreams detected")
	}

	proxy.printProxyTable()
	return proxy
}

func (p *proxy) printProxyTable() {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.AppendHeader(table.Row{"Upstream Group", "Destination"})

	forEachUpstreamGroup(func(upStreamGroup upstreamGroup) {

		var logs []table.Row
		forEachUpstreamHost(func(host string) {
			logs = append(logs, table.Row{upStreamGroup.GetUrlPrefix(), host})
		}, upStreamGroup)

		t.AppendRows(logs)
		t.AppendSeparator()

	}, p.proxies)
	t.Render()
}

func convertUpstream() func(upstreams []*upstream) []balance {
	return upstreamToBalanceGroup
}

// convert all upstream blocks to balancer group
func upstreamToBalanceGroup(upstreams []*upstream) []balance {
	var bGroups []balance
	for _, upStream := range upstreams {
		var upStreamAdds []string
		upStreamAdds = append(upStreamAdds, upStream.Addr)
		bGroup := balance{
			Addr:      upStreamAdds,
			UrlPrefix: upStream.UrlPrefix,
		}
		bGroups = append(bGroups, bGroup)
	}
	return bGroups
}

func (p *proxy) start(close chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		if pHost, pxy := p.getServiceProxy(r.URL.Path); pxy != nil {
			<-time.After(p.delay)
			rspStatus := pxy.RoundTrip(pHost, w,r)
			log.Print(time.Since(now).Microseconds(), "us ", "- [", r.RequestURI, "] ", "\t -> \t", pHost, " ", rspStatus)
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
func (p *proxy) setUpstreamGroup(upStreamGroup upstreamGroup, ctx context.Context, check bool) {
	if check { // ping before connection
		forEachUpstreamHost(func(host string) {
			if checkHostConnection(p.client, host, ctx) {
				p.proxies[upStreamGroup.GetUrlPrefix()] = upStreamGroup // overwrite for each lb
			}
		}, upStreamGroup)
	} else {
		p.proxies[upStreamGroup.GetUrlPrefix()] = upStreamGroup
	}

}
func forEachUpstreamGroup(cb func(upstreamGroup), upstreamGroups map[string]upstreamGroup) {
	for _, upStreamGroup := range upstreamGroups {
		cb(upStreamGroup)
	}
}
func forEachUpstreamHost(cb func(string), upStreamGroup upstreamGroup) {
	for _, upstream := range upStreamGroup.GetHosts() {
		cb(upstream)
	}
}

// GetServiceProxy matches the urlPrefix & returns corresponding reverseProxy
func (p *proxy) getServiceProxy(urlPrefix string) (string, *RTPxy) {
	for proxyUrl, upStreamGroup := range p.proxies {
		if strings.HasPrefix(urlPrefix, proxyUrl) {
			service := *upStreamGroup.GetNext()
			return service.upstream.Addr, &service.pxy
		}
	}
	return "", nil
}
