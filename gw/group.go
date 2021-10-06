package gw

import (
	"sync"
)

type upstreamGroup interface {
	GetUrlPrefix() string
	GetHosts() []string
	GetNext() *service
}
type balanceGroup struct {
	lock     sync.Mutex
	current  int
	group    balance
	services *[]service
}

func (b *balanceGroup) GetHosts() []string {
	var hostAddrs []string
	for _, service := range *b.services {
		hostAddrs = append(hostAddrs, service.upstream.Addr)
	}
	return hostAddrs
}

func (b *balanceGroup) GetUrlPrefix() string {
	return b.group.UrlPrefix
}

func (b *balanceGroup) GetNext() *service {
	b.lock.Lock()
	defer b.lock.Unlock()
	services := *b.services
	service := services[b.current]
	b.current = (b.current + 1) % len(services)
	return &service
}

func new(bGroup balance) upstreamGroup {
	var services []service
	for _, upStream := range bGroup.Addr {
		services = append(services, newService(upstream{
			Name:      "balancer-group-abc",
			Addr:      upStream,
			UrlPrefix: bGroup.UrlPrefix,
		}))
	}
	return &balanceGroup{
		lock:     sync.Mutex{},
		current:  0,
		group:    bGroup,
		services: &services,
	}
}
