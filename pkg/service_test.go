package pkg

import (
	"context"
	"encoding/json"
	"fmt"
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

var _apiGwAddr = ":9000"
var sampleData = map[string]string{"Ping": "Pong"}
var ports1 = []string{"http://localhost:9001", "http://localhost:9002", "http://localhost:9003"}
var ports2 = []string{"http://localhost:9011", "http://localhost:9012", "http://localhost:9013"}

var _apiGw = NewFromConfig(Config{
	Listen:    _apiGwAddr,
	Check:     true,
	Upstreams: nil,
	Balancer:  nil,
})

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
func mockConfig() Config {
	var bg []*balance
	var ug []*Upstream
	bg = append(bg, &balance{
		Addr:      ports1,
		UrlPrefix: "/pong",
	})
	for i, port := range ports2 {
		ug = append(ug, &Upstream{
			Name:      "service" + port,
			Addr:      port,
			UrlPrefix: "/ping" + strconv.Itoa(i),
		})
	}
	return Config{
		Listen:    _apiGwAddr,
		Check:     true,
		Upstreams: ug,
		Balancer:  bg,
	}
}
func buildUpstreams(ctx context.Context) Config {
	c := mockConfig()
	for _, b := range c.Balancer {
		for _, s := range b.Addr {
			go runServer(ctx, s, b.UrlPrefix) // /ping
		}
	}
	for _, u := range c.Upstreams {
		go runServer(ctx, u.Addr, u.UrlPrefix) // /ping1,/ping2,/ping3
	}
	time.After(1 * time.Second)
	_apiGw = NewFromConfig(c)
	return c
}

func testEndpoint(t *testing.T, wg *sync.WaitGroup, endpoint string) {
	<-time.After(50 * time.Millisecond)
	defer wg.Done()
	res, err := http.Get("http://localhost" + _apiGwAddr + endpoint)
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
func Test_ApiGw(t *testing.T) {
	var done = make(chan bool)
	ctx, cancel := context.WithCancel(context.Background())

	gin.SetMode(gin.ReleaseMode)

	c := buildUpstreams(ctx)

	t.Cleanup(func() {
		defer cancel()
		done <- true
	})
	go _apiGw.Start(done)

	<-time.After(3 * time.Second)

	var wg sync.WaitGroup
	for _, b := range c.Balancer {
		for range b.Addr {
			wg.Add(1)
			testEndpoint(t, &wg, b.UrlPrefix)
		}
	}
	for _, u := range c.Upstreams {
		wg.Add(1)
		testEndpoint(t, &wg, u.UrlPrefix)
	}
	wg.Wait()
}
func Test_ApiGwDynamic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	gin.SetMode(gin.ReleaseMode)

	_apiGw = NewFromConfig(Config{
		Listen:    _apiGwAddr,
		Check:     true,
		Upstreams: nil,
		Balancer:  nil,
	})

	defer cancel()

	<-time.After(1 * time.Second)

	var wg sync.WaitGroup
	var ups []Upstream
	send, can := context.WithCancel(context.Background())
	for _, b := range ports1 {
		nu := Upstream{
			Addr:      b,
			UrlPrefix: "/pong",
		}
		ups = append(ups, nu)
		wg.Add(1)
		go runServer(ctx, nu.Addr, nu.UrlPrefix)
		_apiGw.Add(&nu)
		go func(nu Upstream) {
			<-send.Done()
			testEndpoint(t, &wg, nu.UrlPrefix)
		}(nu)
	}
	for _, b := range ports2 {
		nu := Upstream{
			Addr:      b,
			UrlPrefix: "/pong",
		}
		ups = append(ups, nu)
		wg.Add(1)
		go runServer(ctx, nu.Addr, nu.UrlPrefix)
		_apiGw.Add(&nu)
		go func(nu Upstream) {
			<-send.Done()
			testEndpoint(t, &wg, nu.UrlPrefix)
		}(nu)
	}
	can()
	wg.Wait()
	for i, up := range ups {
		if i == len(ups)-1 {
			break
		}
		fmt.Println("\n removing ", up.Addr)
		_apiGw.Remove(&up)
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go testEndpoint(t, &wg, up.UrlPrefix)
		}
		wg.Wait()

	}
}
