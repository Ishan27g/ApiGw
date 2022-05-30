package pkg

import (
	"fmt"
	"sync"
)

type upstreamGroup interface {
	GetUrlPrefix() string
	GetHosts() []string
	GetNext() *service
	AddHost(s *upstream)
	RemoveHost(addr string) (currentSize int)
}
type balanceGroup struct {
	lock      sync.Mutex
	current   int
	urlPrefix string
	services  *[]service
}

func (b *balanceGroup) AddHost(ups *upstream) {
	if ups.UrlPrefix != b.GetUrlPrefix() {
		fmt.Println("s.upstream.UrlPrefix != b.GetUrlPrefix()")
		return
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	s := *b.services
	s = append(s, newService(upstream{
		Name:      "",
		Addr:      ups.Addr,
		UrlPrefix: ups.UrlPrefix,
	}))
	b.services = &s
}
func (b *balanceGroup) RemoveHost(addr string) (currentSize int) {
	b.lock.Lock()
	defer b.lock.Unlock()
	var index = -1
	for i, service := range *b.services {
		if service.upstream.Addr == addr {
			index = i
			break
		}
	}
	if index == -1 {
		return -1
	}
	if index == len(*b.services) {
		b.current = 0
	}
	if index == 0 && len(*b.services) > 1 {
		prv := *b.services
		*b.services = nil
		*b.services = append(*b.services, prv[2:]...)
		fmt.Println("Current upstreams ", *b.services)
		return len(*b.services)
	}
	*b.services = append((*b.services)[:index], (*b.services)[index+1:]...)
	fmt.Println("Current upstreams ", *b.services)
	return len(*b.services)
}

func (b *balanceGroup) GetHosts() []string {
	var hostAddrs []string
	for _, service := range *b.services {
		hostAddrs = append(hostAddrs, service.upstream.Addr)
	}
	return hostAddrs
}

func (b *balanceGroup) GetUrlPrefix() string {
	return b.urlPrefix
}

func (b *balanceGroup) GetNext() *service {
	b.lock.Lock()
	defer b.lock.Unlock()
	services := *b.services
	if len(services) == 0 {
		return nil
	}
	if b.current >= len(services) {
		b.current = 0
	}
	service := services[b.current]
	b.current = (b.current + 1) % len(services)
	return &service
}

func newUpsGroup(bGroup balance) upstreamGroup {
	var services []service
	for _, upStream := range bGroup.Addr {
		services = append(services, newService(upstream{
			Name:      "balancer-group-abc",
			Addr:      upStream,
			UrlPrefix: bGroup.UrlPrefix,
		}))
	}
	return &balanceGroup{
		lock:      sync.Mutex{},
		current:   0,
		urlPrefix: bGroup.UrlPrefix,
		services:  &services,
	}
}
