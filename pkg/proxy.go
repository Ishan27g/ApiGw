package pkg

import (
	"context"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

func (p *RTPxy) RoundTrip(ctx context.Context, span trace.Span, rw http.ResponseWriter, req *http.Request) int {
	return roundTrip(ctx, span, p.hostAddress, rw, req)
}
func roundTrip(ctx context.Context, span trace.Span, host string, rw http.ResponseWriter, req *http.Request) int {
	outReq, _ := http.NewRequestWithContext(ctx, req.Method, host+req.RequestURI, req.Body)
	// span := trace.SpanFromContext(outReq.Context())
	for key, value := range req.Header {
		for _, v := range value {
			outReq.Header.Add(key, v)
			span.SetAttributes(attribute.Key("REQUEST_" + key).String(v))
		}
	}
	span.AddEvent("proxy_begin_RT at " + time.Now().String())
	res, err := otelhttp.NewTransport(defaultTransport).RoundTrip(outReq)
	//res, err := defaultTransport.RoundTrip(outReq)
	span.AddEvent("proxy_end_RT at " + time.Now().String())
	if err != nil {
		rw.WriteHeader(http.StatusBadGateway)
		return http.StatusBadGateway
	}
	for key, value := range res.Header {
		for _, v := range value {
			rw.Header().Add(key, v)
			span.SetAttributes(attribute.Key("RESPONSE_" + key).String(v))
		}
	}
	span.SetAttributes(attribute.Key("RESPONSE_Status").Int(res.StatusCode))
	rw.WriteHeader(res.StatusCode)
	io.Copy(rw, res.Body)
	res.Body.Close()
	return res.StatusCode
}
