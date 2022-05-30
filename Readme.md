### Define a configuration `.hcl` file

```hcl
listen = ":9000" # address for gateway
checkConnections = true # optional, default false. Ping upstreams before start
addDelay = "2s" # optional, default 0s

balance { # load balance
  addr = [
    "http://localhost:7001",
    "http://localhost:7002",
    "http://localhost:7003"
  ]
  urlPrefix = "/status"         # request url prefix match
}

upstream "service-1" {
  addr = "http://localhost:5001" # destination to route the request
  urlPrefix = "/prefix1"         # request url prefix match 
}

upstream "service-2" {
  addr = "http://localhost:5002"
  urlPrefix = "/prefix2"
}

upstream "service-3" {
  addr = "http://localhost:5003"
  urlPrefix = "/prefix/3/2/1"
}
```

### Usage

Can be used as a package, or as cli tool

#### CLI

Commands
- { check } { file.hcl }
- { start } { file.hcl }

Either install globally as a cli

```shell
# cd ApiGw/
$ go install
$ pkg --help
$ pkg {command} {hcl-file}
```

Or, without installing

```shell
go run main.go {command} {hcl-file}
```

#### Import as library

```go
package main

import "github.com/Ishan27g/ApiGw/pkg"

func main() {
	stopGateway := make(chan bool, 1)

	go apiGw.NewFromFile("conf.hcl").Start(stopGateway)
	// stopGateway <- true
}
```