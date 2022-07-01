package pkg

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	Listen    string      `hcl:"listen"`
	Delay     string      `hcl:"addDelay,optional"`
	Check     bool        `hcl:"checkConnections,optional"`
	Upstreams []*Upstream `hcl:"Upstream,block"`
	Balancer  []*balance  `hcl:"balance,block"`
}

type balance struct {
	Addr      []string `hcl:"addr"`
	UrlPrefix string   `hcl:"urlPrefix"`
	names     []string
}
type Upstream struct {
	Name      string `hcl:",label"`
	Addr      string `hcl:"addr"`
	UrlPrefix string `hcl:"urlPrefix"`
}

func ReadConfFile(filename string) (Config, error) {
	var config Config
	err := hclsimple.DecodeFile(filename, nil, &config)
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
		return Config{}, err
	}
	if config.Delay == "" {
		config.Delay = "0s"
	}
	return config, errors.New(filename + " - Ok")
}

var checkHostConnection = func(client http.Client, host string, ctx context.Context) bool {
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
