package apiGw

import (
    "context"
    "encoding/json"
    "io"
    "log"
    "net/http"
    "strconv"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
)
var apiGwAddr = ":9000"
var sampleData = map[string]string{"Ping": "Pong"}
var ports1 = []string{"http://localhost:9001", "http://localhost:9002", "http://localhost:9003"}
var ports2 = []string{"http://localhost:9011", "http://localhost:9012", "http://localhost:9013"}

func runServer(ctx context.Context, addr, endpoint string) {
    addr = strings.Trim(addr, "http://")
    r := gin.New()
    r.Use(gin.Logger())
    r.Use(gin.Recovery())
    r.Any(endpoint, func(c *gin.Context) {
        c.JSON(http.StatusOK, sampleData)
    })
    httpSrv := &http.Server{
        Addr:    addr,
        Handler: r,
    }
    go func() {
        log.Println("listening on", httpSrv.Addr)
        if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Println("Error listening on", httpSrv.Addr, err.Error())
        }
    }()
    <-ctx.Done()
    cx, can := context.WithTimeout(context.Background(), 3*time.Second)
    defer can()
    if err := httpSrv.Shutdown(cx); err != nil {
        log.Println("Error shutting down on", httpSrv.Addr, err.Error())
    }
}
func mockConfig()Config {
    var bg []*balance
    var ug []*upstream
    bg = append(bg, &balance{
        Addr:      ports1,
        UrlPrefix: "/pong",
    })
    for i, port := range ports2 {
        ug = append(ug, &upstream{
            Name:      "service" + port,
            Addr:      port,
            UrlPrefix: "/ping" + strconv.Itoa(i),
        })
    }
    return Config{
        Listen:    apiGwAddr,
        Check:     true,
        Upstreams: ug,
        Balancer:  bg,
    }
}
func buildUpstreams(ctx context.Context) (ApiGw, Config){
    c := mockConfig()
    for _, b := range c.Balancer {
        for _, s := range b.Addr {
            go runServer(ctx, s, b.UrlPrefix) // /ping
        }
    }
    for _, u := range c.Upstreams {
        go runServer(ctx, u.Addr, u.UrlPrefix) // /ping1,/ping2,/ping3
    }
    return NewFromConfig(c), c
}


func testEndpoint(t *testing.T, wg *sync.WaitGroup, endpoint string) {
    defer wg.Done()
    res, err := http.Get("http://localhost" + apiGwAddr + endpoint)
    if err != nil {
        log.Fatal(err)
    }
    response, err := io.ReadAll(res.Body)
    res.Body.Close()
    if err != nil {
        log.Fatal(err)
    }
    assert.Equal(t, 200, res.StatusCode)
    var rd map[string]string
    err = json.Unmarshal(response, &rd)
    assert.NoError(t, err)
    assert.Equal(t, sampleData, rd)
}
func TestApiGw(t *testing.T){
    var done = make(chan bool)
    ctx, cancel := context.WithCancel(context.Background())

    gin.SetMode(gin.ReleaseMode)

    apiGw, c := buildUpstreams(ctx)

    t.Cleanup(func() {
        defer cancel()
        done <- true
    })

    apiGw.Start(done)

    <- time.After(3 * time.Second)

    var wg sync.WaitGroup
    for _, b := range c.Balancer {
        for range b.Addr{
            wg.Add(1)
            testEndpoint(t, &wg, b.UrlPrefix)
        }
    }
    for _, u := range c.Upstreams {
        wg.Add(1)
        testEndpoint(t, &wg, u.UrlPrefix)
    }
}
