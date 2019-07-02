package service

import (
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"context"
	mcp "istio.io/api/mcp/v1alpha1"
	"github.com/gogo/protobuf/proto"
)

func (s *AdsService) EstablishResourceStream(mcps mcp.ResourceSource_EstablishResourceStreamServer) error {
	return s.stream(&mcpStream{stream: mcps})
}

type mcpStream struct {
	stream mcp.ResourceSource_EstablishResourceStreamServer
}

func (mcps *mcpStream) Send(p proto.Message) error {
	if mp, ok := p.(*mcp.Resources); ok {
		return mcps.stream.Send(mp)
	}
	return errors.New("Invalid stream")
}

func (mcps *mcpStream) Recv() (proto.Message, error) {
	p, err := mcps.stream.Recv()

	if err != nil {
		return nil, err
	}

	return p, err
}
func (mcps *mcpStream) Context() context.Context {
	return context.Background()
}

// Compared with ADS:
//  req.Node -> req.SinkNode
//  metadata struct -> Annotations
//  TypeUrl -> Collection
//  no on-demand (Watched)
func (mcps *mcpStream) Process(s *AdsService, con *Connection, msg proto.Message) error {
	req := msg.(*mcp.RequestResources)
	if !con.active {
		var id string
		if req.SinkNode == nil || req.SinkNode.Id == "" {
			log.Println("Missing node id ", req.String())
			id = con.PeerAddr
		} else {
			id = req.SinkNode.Id
		}

		con.mu.Lock()
		con.NodeID = id
		con.Metadata = req.SinkNode.Annotations
		con.ConID = s.connectionID(con.NodeID)
		con.mu.Unlock()

		s.mutex.Lock()
		s.clients[con.ConID] = con
		s.mutex.Unlock()

		con.active = true
	}

	rtype := req.Collection

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

	// Blocking - read will continue
	err := s.push(con, rtype, nil)
	if err != nil {
		// push failed - disconnect
		log.Println("Closing connection ", err)
		return err
	}

	return err
}

