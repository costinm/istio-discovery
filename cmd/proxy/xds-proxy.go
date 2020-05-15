package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/costinm/istio-discovery/pkg/service"
)

var (
	server = flag.String("up", "127.0.0.1:8500", "Address of upstream XDS server")

	grpcAddr = flag.String("grpcAddr", ":15098", "Address of the local ADS/MCP server")

	addr = flag.String("httpAddr", ":15099", "Address of the HTTP debug server")

	dir = flag.String("static", ".", "Location of local files, merged with the upstream server")
)

// Minimal MCP/ADS proxy
// Will use KUBECONFIG or in-cluster config for k8s
func main() {
	flag.Parse()

	a := service.NewService(*grpcAddr)

	// TODO: multiple ADS clients

	log.Println("Starting", a)

	http.ListenAndServe(*addr, nil)
}
