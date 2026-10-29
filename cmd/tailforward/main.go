package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"tailforward/internal/browser"
	"tailforward/internal/config"
	"tailforward/internal/domain"
	"tailforward/internal/httpapi"
	"tailforward/internal/service"
	"tailforward/internal/state"
)

//goland:noinspection HttpUrlsUsage
func main() {
	noBrowser := flag.Bool("no-browser", false, "do not open the browser automatically on launch")
	flag.Parse()
	paths, err := config.FromExecutable()
	if err != nil {
		log.Fatal(err)
	}
	tracker := state.New("starting")
	svc := service.New(paths, tracker)
	svc.Restore()
	go svc.Loop()

	addr := fmt.Sprintf("127.0.0.1:%d", domain.DashboardPort)
	var ln net.Listener
	for attempt := 1; ; attempt++ {
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		if attempt >= 20 {
			log.Fatal(err)
		}
		log.Printf("dashboard port busy, retrying: %v", err)
		time.Sleep(500 * time.Millisecond)
	}
	if !*noBrowser {
		go browser.Open("http://" + addr + "/")
	}
	log.Printf("tailforward on http://%s", addr)
	server := &http.Server{
		Handler:           httpapi.New(svc).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(server.Serve(ln))
}
