# Istio ports

Start of a document listing all ports used in Istio.

With 'whitebox' mode it is useful to use unique ports for each service, and
not overlap with Istio services which may be used across namespaces.

Some of the ports can be configured - but most testing is done with the default 
values. Changing the defaults may or may not work, it is not recommended for 
most users.

## Per pod ports

- 127.0.0.1:15000 - envoy admin port
- 15001 - iptables REDIRECT or TPROXY capture
- 127.0.0.1:15002 - HTTP PROXY (in whitebox mode or if globally enabled)
- 15090 - Envoy metrics
- 15020 - Pilot agent liveness

## Control plane

- 15010 - Pilot GPRC
- 15011 - Pilot GRPC Secure
-       - Citadel GRPC
-       
-       - Pilot metrics / admin
-       - Mixer report and policy - different VIPs
-       - Galley

## Common services/URLs

Common port:

- /metrics: pulled from prometheus, can be used for debugging 
- /ctrlz: UI



