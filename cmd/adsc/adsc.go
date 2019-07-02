// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

// Single client

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/costinm/istio-discovery/pkg/adsc"
)

var (

	pilotAddr = flag.String("pilot",
		"localhost:15010",
		"Pilot address. Can be a real pilot exposed for mesh expansion.")

	port = flag.String("port",
		":14100",
		"Http port for debug and control.")

	routerType = flag.Bool("router",
		false,
		"Node type will be router")

	namespace = flag.String("n",
		"test",
		"Namespace")

	certDir = flag.String("certDir",
		"", // /etc/certs",
		"Certificate dir. Must be set according to mesh expansion docs for testing a meshex pilot.")
)

func main() {
	flag.Parse()

	go runClient(0)
	http.Handle("/metrics", promhttp.Handler())
	initMetrics()
	err := http.ListenAndServe(*port, nil)
	if err != nil {
		log.Fatal("failed to start monitoring port ", err)
	}
}

var (
	initialConnectz = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "connect",
		Help:    "Initial connection time, in ms",
		Buckets: []float64{100, 500, 1000, 2000, 5000, 10000, 20000, 40000},
	})

	connectedz = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "connected",
		Help: "Connected clients",
	})

	connectTimeoutz = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "connectTimeout",
		Help: "Connect timeouts",
	})

	cfgRecvz = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "configs",
		Help: "Received config types",
	}, []string{"type"})
)

func initMetrics() {
	prometheus.MustRegister(initialConnectz)
	prometheus.MustRegister(connectedz)
	prometheus.MustRegister(connectTimeoutz)
	prometheus.MustRegister(cfgRecvz)
}

// runClient creates a single long lived connection
func runClient(n int) {
	cfg := &adsc.AdsConfig{
		IP: net.IPv4(10, 10, byte(n/256), byte(n%256)).String(),
		Namespace: *namespace,
		Meta: map[string]string {
			"INTERCEPTION_MODE": "NONE",
		},
	}
	if *routerType {
		cfg.NodeType = "router"

	}
	c, err := adsc.Dial(*pilotAddr, *certDir, cfg)
	if err != nil {
		log.Println("Error connecting ", err)
		return
	}
	c.DumpCfg = true

	t0 := time.Now()

	c.Watch()

	initialConnect := true
	_, err = c.Wait("rds", 30*time.Second)
	if err != nil {
		log.Println("Timeout receiving RDS")
		connectTimeoutz.Add(1)
		initialConnect = false
	} else {
		connectedz.Add(1)
	}

	ctime := time.Since(t0)
	initialConnectz.Observe(float64(ctime / 1000000)) // ms

	for {
		msg, err := c.Wait("", 15*time.Second)
		if err == adsc.ErrTimeout {
			continue
		}
		if msg == "close" {
			//err = c.Reconnect()
			//if err != nil {
				log.Println("Failed to reconnect")
				return
//			}
		}
		if !initialConnect && msg == "rds" {
			// This is a delayed initial connect
			connectedz.Add(1)
			initialConnect = true
		}
		cfgRecvz.With(prometheus.Labels{"type": msg}).Add(1)
		log.Println("Received ", msg)
	}
}
