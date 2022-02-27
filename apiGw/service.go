package apiGw

import (
	"io"
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
type RTPxy struct {}

type service struct {
	upstream upstream
	pxy RTPxy
}
func newService(upstream upstream) service {
	return service{
		upstream: upstream,
		pxy: RTPxy{},
	}
}

func (p *RTPxy) RoundTrip(host string, rw http.ResponseWriter, req *http.Request) int {
	outReq, _ := http.NewRequest(req.Method, host + req.RequestURI, req.Body)
	for key, value := range req.Header {
		for _, v := range value {
			outReq.Header.Add(key, v)
		}
	}
	res, err := defaultTransport.RoundTrip(outReq)
	if err != nil {
		rw.WriteHeader(http.StatusBadGateway)
		return http.StatusBadGateway
	}
	for key, value := range res.Header {
		for _, v := range value {
			rw.Header().Add(key, v)
		}
	}
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
	res.Body.Close()
	return res.StatusCode
}