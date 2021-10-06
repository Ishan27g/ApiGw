package gw

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	RequestMaxWaitTime = 5 * time.Second
)

type ApiGw interface {
	// Start api-gw, non-blocking
	Start(close chan bool)
	// RegisterService registers a host to redirect requests that match the urlPrefix
	setUpstreamGroup(upStreamGroup upstreamGroup, ctx context.Context, check bool, t table.Writer)
	// GetServiceProxy returns the http.ReverseProxy for the host that matched the urlPrefix
	getServiceProxy(urlPrefix string) (string, *httputil.ReverseProxy)
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

	for _, group := range config.Balancer {
		g := *group
		upStreamGroups = append(upStreamGroups, new(g))
	}

	bGroup := convertUpstream()(config.Upstreams)
	for _, group := range bGroup {
		upStreamGroups = append(upStreamGroups, new(group))
	}
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.AppendHeader(table.Row{"Upstream Group", "Destination"})

	for _, upStreamGroup := range upStreamGroups {
		proxy.setUpstreamGroup(upStreamGroup, ctx, config.Check, t)
		t.AppendSeparator()
	}
	t.Render()
	if len(proxy.proxies) == 0 {
		log.Fatalln("no upstreams detected")
	}
	/*
		for _, upStreamGroup := range proxy.proxies {
			for _, upstream := range upStreamGroup.GetHosts() {
				log.Print(upStreamGroup.GetUrlPrefix(), "\t-> \t", upstream)
			}
		}
	*/
	return proxy
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
func (p *proxy) setUpstreamGroup(upStreamGroup upstreamGroup, ctx context.Context, check bool, t table.Writer) {
	// if check {
	// if checkHostConnection(p.client, upstreamGroup(), ctx) {
	p.proxies[upStreamGroup.GetUrlPrefix()] = upStreamGroup
	var logs []table.Row
	for _, upstream := range upStreamGroup.GetHosts() {
		logs = append(logs, table.Row{upStreamGroup.GetUrlPrefix(), upstream})
	}
	t.AppendRows(logs)
	// }
	// } else {
	// p.proxies[upstream.UrlPrefix] = newService(upstream)
	// }
}

// GetServiceProxy matches the urlPrefix & returns corresponding reverseProxy
func (p *proxy) getServiceProxy(urlPrefix string) (string, *httputil.ReverseProxy) {
	for proxyUrl, upStreamGroup := range p.proxies {
		if strings.HasPrefix(urlPrefix, proxyUrl) {
			service := *upStreamGroup.GetNext()
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
