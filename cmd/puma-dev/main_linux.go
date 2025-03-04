package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/puma/puma-dev/dev"
	"github.com/puma/puma-dev/homedir"
)

var (
	fDebug              = flag.Bool("debug", false, "enable debug output")
	fDir                = flag.String("dir", "~/.puma-dev", "directory to watch for apps")
	fDomains            = flag.String("d", "test", "domains to handle, separate with :, defaults to test")
	fHTTPPort           = flag.Int("http-port", 9280, "port to listen on http for")
	fNoServePublicPaths = flag.String("no-serve-public-paths", "", "Disable static file server for specific paths under /public")
	fStop               = flag.Bool("stop", false, "Stop all puma-dev servers")
	fSysBind            = flag.Bool("sysbind", false, "bind to ports 80 and 443")
	fTimeout            = flag.Duration("timeout", 15*60*time.Second, "how long to let an app idle for")
	fTLSPort            = flag.Int("https-port", 9283, "port to listen on https for")
)

func main() {
	flag.Parse()

	allCheck()

	domains := strings.Split(*fDomains, ":")

	if *fStop {
		err := dev.Stop()
		if err != nil {
			log.Fatalf("Unable to stop puma-dev servers: %s", err)
		}
		return
	}

	if *fSysBind {
		*fHTTPPort = 80
		*fTLSPort = 443
	}

	dir, err := homedir.Expand(*fDir)
	if err != nil {
		log.Fatalf("Unable to expand dir: %s", err)
	}

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatalf("Unable to create dir '%s': %s", dir, err)
	}

	var events dev.Events

	var pool dev.AppPool
	pool.Dir = dir
	pool.IdleTime = *fTimeout
	pool.Events = &events

	purge := make(chan os.Signal, 1)
	signal.Notify(purge, syscall.SIGUSR1)

	go func() {
		for {
			<-purge
			pool.Purge()
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)

	go func() {
		<-shutdown
		fmt.Printf("! Shutdown requested\n")
		pool.Purge()
		os.Exit(0)
	}()

	err = dev.SetupOurCert()
	if err != nil {
		log.Fatalf("Unable to setup TLS cert: %s", err)
	}

	fmt.Printf("* Directory for apps: %s\n", dir)
	fmt.Printf("* Domains: %s\n", strings.Join(domains, ", "))
	fmt.Printf("* HTTP Server port: %d\n", *fHTTPPort)
	fmt.Printf("* HTTPS Server port: %d\n", *fTLSPort)

	var http dev.HTTPServer

	http.Address = fmt.Sprintf(":%d", *fHTTPPort)
	http.TLSAddress = fmt.Sprintf(":%d", *fTLSPort)
	http.Pool = &pool
	http.Debug = *fDebug
	http.Events = &events
	http.Domains = domains
	if len(*fNoServePublicPaths) > 0 {
		http.IgnoredStaticPaths = strings.Split(*fNoServePublicPaths, ":")
		fmt.Printf("* Ignoring files under: public{%s}\n", strings.Join(http.IgnoredStaticPaths, ", "))
	}

	http.Setup()

	fmt.Printf("! Puma dev listening on http and https\n")

	go http.ServeTLS()

	err = http.Serve()
	if err != nil {
		log.Fatalf("Error listening: %s", err)
	}
}
