package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"istio.io/api/mcp/v1alpha1"
)

// Main implementation of the XDS, MCP and SDS services, using a common internal structures and model.

var (
	nacks = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "xds_nack",
		Help: "Nacks.",
	}, []string{"node", "type"})

	acks = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "xds_ack",
		Help: "Aacks.",
	}, []string{"type"})

	// key is the XDS/MCP type
	resourceHandler = map[string]TypeHandler{}
)

// TypeHandler is called when a request for a type is first received.
// It should send the list of resources on the connection.
type TypeHandler func(s *AdsService, con *Connection, rtype string, res []string) error

// AdsService implements ADS, MCP, SDS (and possibly other bi-directional streams of objects)
type AdsService struct {
	grpcServer *grpc.Server

	// mutex used to modify structs, non-blocking code only.
	mutex sync.RWMutex

	// clients reflect active gRPC channels, for both ADS and MCP.
	// key is Connection.ConID
	clients map[string]*Connection

	connectionNumber int
}

// Connection represents a single endpoint.
// An endpoint typically has 0 or 1 connections - but during restarts and drain it may have >1.
type Connection struct {
	mu sync.RWMutex

	// PeerAddr is the address of the client envoy, from network layer
	PeerAddr string

	NodeID string

	// Time of connection, for debugging
	Connect time.Time

	// ConID is the connection identifier, used as a key in the connection table.
	// Currently based on the node name and a counter.
	ConID string

	// doneChannel will be closed when the client is closed.
	doneChannel chan int

	// Metadata key-value pairs extending the Node identifier
	Metadata map[string]string

	// Watched resources for the connection
	Watched map[string][]string

	NonceSent  map[string]string
	NonceAcked map[string]string

	// Only one can be set.
	Stream Stream

	active     bool
	resChannel chan proto.Message
	errChannel chan error
}

// NewService initialized the grpc services. Non-blocking, returns error if it can't listen on the address.
func NewService(addr string) *AdsService {
	adss := &AdsService{
		clients: map[string]*Connection{},
	}

	adss.initGrpcServer()

	ads.RegisterAggregatedDiscoveryServiceServer(adss.grpcServer, adss)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	go adss.grpcServer.Serve(lis)

	return adss
}

// Streams abstracts the protocol details. Generic Message is used for sending and receiving.
type Stream interface {
	// Send will push a message.
	// Message can be mcp.Resources, v2.DiscoveryResponse, sds, etc.
	Send(proto.Message) error

	// Recv is used by the main stream process goroutine to get requests from the endpoint.
	// Can be mcp.RequestResources ,v2.DiscoveryRequest
	Recv() (proto.Message, error)

	// Context returns the context for the connection, for metadata.
	Context() context.Context

	// Process is called by the stream processing goroutine on each received message.
	// Should not block - no new Recv will be called while Process is in progress.
	Process(s *AdsService, con *Connection, message proto.Message) error
}

func AddHandler(typ string, handler TypeHandler) {
	log.Println("HANDLER: ", typ)
	resourceHandler[typ] = handler
}

// Process one 'stream' - can be XDS, MCP, SDS
func (s *AdsService) stream(stream Stream) error {

	peerInfo, ok := peer.FromContext(stream.Context())
	peerAddr := "0.0.0.0"
	if ok {
		peerAddr = peerInfo.Addr.String()
	}

	t0 := time.Now()

	con := &Connection{
		Connect:     t0,
		PeerAddr:    peerAddr,
		Stream:      stream,
		NonceSent:   map[string]string{},
		Metadata:    map[string]string{},
		Watched:     map[string][]string{},
		NonceAcked:  map[string]string{},
		doneChannel: make(chan int, 2),
		resChannel:  make(chan proto.Message, 2),
		errChannel:  make(chan error, 2),
	}
	// Unlike pilot, this uses the more direct 'main thread handles read' mode.
	// It also means we don't need 2 goroutines per connection.

	firstReq := true

	defer func() {
		if firstReq {
			return // didn't get first req, not added
		}
		close(con.resChannel)
		close(con.doneChannel)
		s.mutex.Lock()
		delete(s.clients, con.ConID)
		s.mutex.Unlock()

	}()

	go func() {
		for {
			// Blocking. Separate go-routines may use the stream to push.
			req, err := stream.Recv()
			if err != nil {
				if status.Code(err) == codes.Canceled || err == io.EOF {
					log.Printf("ADS: %q %s terminated %v", con.PeerAddr, con.ConID, err)
					con.errChannel <- nil
					return
				}
				log.Printf("ADS: %q %s terminated with errors %v", con.PeerAddr, con.ConID, err)
				con.errChannel <- err
				return
			}
			err = stream.Process(s, con, req)
			if err != nil {
				con.errChannel <- err
				return
			}
		}
	}()

	for {
		select {
		case res, _ := <-con.resChannel:
			err := stream.Send(res)
			if err != nil {
				return err
			}
		case err1, _ := <-con.errChannel:
			return err1
		}
	}

	return nil
}

// Push a single resource type on the connection. This is blocking.
func (s *AdsService) push(con *Connection, rtype string, res []string) error {
	h, f := resourceHandler[rtype]
	if !f {
		// TODO: push some 'not found'
		log.Println("Resource not found ", rtype)
		r := &v1alpha1.Resources{}
		r.Collection = rtype
		s.Send(con, rtype, r)
		return nil

	}
	return h(s, con, rtype, res)
}

func (fx *AdsService) SendAll(r *v1alpha1.Resources) {
	for _, con := range fx.clients {
		// TODO: only if watching our resource type

		r.Nonce = fmt.Sprintf("%v", time.Now())
		con.NonceSent[r.Collection] = r.Nonce
		con.Stream.Send(r)
	}

}

func (fx *AdsService) Send(con *Connection, rtype string, r *v1alpha1.Resources) error {
	r.Nonce = fmt.Sprintf("%v", time.Now())
	con.NonceSent[rtype] = r.Nonce
	return con.Stream.Send(r)
}
