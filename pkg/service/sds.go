package service

import (
	"context"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
)

func (s *AdsService) StreamSecrets(ads.SecretDiscoveryService_StreamSecretsServer) error {
	return nil
}

func (s *AdsService) FetchSecrets(context.Context, *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	return nil, nil
}

func (s *AdsService) DeltaSecrets(mcps ads.SecretDiscoveryService_DeltaSecretsServer) error {
	return nil
}


