package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/costinm/istio-discovery/pkg/service"
)

var (
	server = flag.String("consul", "127.0.0.1:8500", "Address of consul agent")

	grpcAddr = flag.String("grpcAddr", ":15098", "Address of the ADS/MCP server")

	addr = flag.String("httpAddr", ":15099", "Address of the HTTP debug server")
)

// Minimal MCP server exposing k8s and consul synthetic entries
// Currently both are returned to test the timing of the k8s-to-consul sync
// Will use KUBECONFIG or in-cluster config for k8s
func main() {
	flag.Parse()

	a := service.NewService(*grpcAddr)

	log.Println("Starting", a)

	http.ListenAndServe(*addr, nil)
}

