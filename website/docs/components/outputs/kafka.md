---
title: kafka
type: output
---

```yaml
kafka: {}
```

The kafka output type writes a batch of messages to Kafka brokers and waits for
acknowledgement before propagating it back to the input. The config field
`ack_replicas` determines whether we wait for acknowledgement from all
replicas or just a single broker.

It is possible to specify a compression codec to use out of the following
options: `none`, `snappy`, `lz4` and `gzip`.

Both the `key` and `topic` fields can be dynamically set using
function interpolations described [here](../config_interpolation.md#functions).
When sending batched messages these interpolations are performed per message
part.

The `partitioner` field determines how messages are delegated to a
partition. Options are `fnv1a_hash`, `murmur2_hash`, `random` and
`round_robin`. When a hash partitioner is selected but a message key
is empty then a random partition is chosen.

### TLS

Custom TLS settings can be used to override system defaults. This includes
providing a collection of root certificate authorities, providing a list of
client certificates to use for client verification and skipping certificate
verification.

Client certificates can either be added by file or by raw contents:

``` yaml
enabled: true
client_certs:
  - cert_file: ./example.pem
    key_file: ./example.key
  - cert: foo
    key: bar
```
