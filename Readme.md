### Define a configuration `.hcl` file

```hcl
listen = ":9000" # address for gateway
checkConnections = true # optional, default false. Ping upstreams before start
addDelay = "2s" # optional, default 0s

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

```go
package main

import "github.com/Ishan27g/apiGw"

func main() {
    stop := make(chan bool, 1)

    gw := apiGw.NewFromFile("conf.hcl")

    gw.Start(stop)
    
    // stop <- true
}
```
