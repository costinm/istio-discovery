package service

import (
	"context"
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
)

// IncrementalAggregatedResources is not implemented.
func (s *AdsService) DeltaAggregatedResources(stream ads.AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error {
	return status.Errorf(codes.Unimplemented, "not implemented")
}

// StreamAggregatedResources implements the Envoy variant. Can be used directly with EDS.
func (s *AdsService) StreamAggregatedResources(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	return s.stream(&adsStream{stream: stream})
}


type adsStream struct {
	stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer
}

func (mcps *adsStream) Send(p proto.Message) error {
	if mp, ok := p.(*v2.DiscoveryResponse); ok {
		return mcps.stream.Send(mp)
	}
	return errors.New("Invalid stream")
}


func (mcps *adsStream) Recv() (proto.Message, error) {
	p, err := mcps.stream.Recv()

	if err != nil {
		return nil, err
	}

	return p, err
}

func (mcps *adsStream) Context() context.Context {
	return context.Background()
}

func (mcps *adsStream) Process(s *AdsService, con *Connection, msg proto.Message) error {
	req := msg.(*v2.DiscoveryRequest)
	if !con.active {
		if req.Node == nil || req.Node.Id == "" {
			log.Println("Missing node id ", req.String())
			return errors.New("Missing node id")
		}

		con.mu.Lock()
		con.NodeID = req.Node.Id
		con.Metadata = parseMetadata(req.Node.Metadata)
		con.ConID = s.connectionID(con.NodeID)
		con.mu.Unlock()

		s.mutex.Lock()
		s.clients[con.ConID] = con
		s.mutex.Unlock()

		con.active = true
	}

	rtype := req.TypeUrl

	if req.ErrorDetail != nil && req.ErrorDetail.Message != "" {
		nacks.With(prometheus.Labels{"node": con.NodeID, "type": rtype}).Add(1)
		log.Println("NACK: ", con.NodeID, rtype, req.ErrorDetail)
		return nil
	}
	if req.ErrorDetail != nil && req.ErrorDetail.Code == 0 {
		con.mu.Lock()
		con.NonceAcked[rtype] = req.ResponseNonce
		con.mu.Unlock()
		acks.With(prometheus.Labels{"type": rtype}).Add(1)
		return nil
	}

	if req.ResponseNonce != "" {
		// This shouldn't happen
		con.mu.Lock()
		lastNonce := con.NonceSent[rtype]
		con.mu.Unlock()

		if lastNonce == req.ResponseNonce {
			acks.With(prometheus.Labels{"type": rtype}).Add(1)
			con.mu.Lock()
			con.NonceAcked[rtype] = req.ResponseNonce
			con.mu.Unlock()
			return nil
		} else {
			// will resent the resource, set the nonce - next response should be ok.
			log.Println("Unmatching nonce ", req.ResponseNonce, lastNonce)
		}
	}

	con.mu.Lock()
	// TODO: find added/removed resources, push only those.
	con.Watched[rtype] = req.ResourceNames
	con.mu.Unlock()

	// Blocking - read will continue
	err := s.push(con, rtype, req.ResourceNames)
	if err != nil {
		// push failed - disconnect
		return err
	}

	return err
}

