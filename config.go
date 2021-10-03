package apiGw

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	Listen    string      `hcl:"listen"`
	Delay     string      `hcl:"addDelay,optional"`
	Check     bool        `hcl:"checkConnections,optional"`
	Upstreams []*upstream `hcl:"upstream,block"`
}
type upstream struct {
	Name      string `hcl:",label"`
	Addr      string `hcl:"addr"`
	UrlPrefix string `hcl:"urlPrefix"`
}

func read(filename string) Config {
	var config Config
	err := hclsimple.DecodeFile(filename, nil, &config)
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
	}
	if config.Delay == "" {
		config.Delay = "0s"
	}
	return config
}

func checkHostConnection(client http.Client, host string, ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host, nil)
	if err != nil {
		log.Panicf("Cannot create request: %s\n", err)
	}
	rsp, err := client.Do(req)
	if rsp != nil {
		defer rsp.Body.Close()
	}
	if e, ok := err.(net.Error); ok && e.Timeout() {
		log.Println("request timed out: ", err)
		return false
	} else if err != nil {
		log.Println("Error in sending request: ", err)
		return false
	}
	log.Println("Connected to ", host)
	return true
}
