package main

import (
	"flag"
	"log"
)

var (
	configFile = flag.String("mappingConfig", "mist.conf", "The file that contains hostname to endpoint mappings")
	listenAddr = flag.String("listenAddr", ":80", "The address and port that mist should listen on for incoming connections")
)

func main() {
	flag.Parse()
	p := NewHostProxy()
	if err := p.LoadMappingsFrom(*configFile); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	p.ListenAndServe(*listenAddr)
}
