package pkg

import (
	"io"
	"net/http"
)

type RTPxy struct {
	hostAddress, urlPrefix string
}

func newProxy(host, urlPrefix string) *RTPxy {
	return &RTPxy{
		hostAddress: host,
		urlPrefix:   urlPrefix,
	}
}

func (p *RTPxy) RoundTrip(rw http.ResponseWriter, req *http.Request) int {
	return roundTrip(p.hostAddress, rw, req)
}
func roundTrip(host string, rw http.ResponseWriter, req *http.Request) int {
	outReq, _ := http.NewRequest(req.Method, host+req.RequestURI, req.Body)
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
