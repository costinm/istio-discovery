#!/usr/bin/env bash

func initConsul() {
    # TODO: if missing
    go get github.com/hashicorp/consul

    # http://localhost:8500/ui
    consul agent -dev -config-dir=$TOP/src/github.com/costinm/istio-discovery/pkg/consul/testdata/config-dir -ui


    helm install -n consul --namespace consul . --set client.grpc=true --set syncCatalog.enabled=true --set connectInject.enabled=true

    kubectl --namespace consul port-forward $(kubectl -n consul get -l app=consupod -l component=server -o=jsonpath='{.items[0].metadata.name}') 8501:8500

# https://www.consul.io/docs/platform/k8s/helm.html#configuration-values-

}

func testData() {
    curl http://localhost:8500/v1/catalog/service/a

    curl \
    --request PUT \
    --data @pkg/consul/testdata/svcRegister.json \
    http://127.0.0.1:8500/v1/catalog/register

    consul
}
