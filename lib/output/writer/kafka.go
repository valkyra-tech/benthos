package writer

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/message"
	"github.com/Jeffail/benthos/v3/lib/message/batch"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/types"
	"github.com/Jeffail/benthos/v3/lib/util/hash/murmur2"
	"github.com/Jeffail/benthos/v3/lib/util/retries"
	"github.com/Jeffail/benthos/v3/lib/util/text"
	btls "github.com/Jeffail/benthos/v3/lib/util/tls"
	"github.com/Shopify/sarama"
	"github.com/cenkalti/backoff"
)

//------------------------------------------------------------------------------

// KafkaConfig contains configuration fields for the Kafka output type.
type KafkaConfig struct {
	Addresses      []string    `json:"addresses" yaml:"addresses"`
	ClientID       string      `json:"client_id" yaml:"client_id"`
	Key            string      `json:"key" yaml:"key"`
	Partitioner    string      `json:"partitioner" yaml:"partitioner"`
	Topic          string      `json:"topic" yaml:"topic"`
	Compression    string      `json:"compression" yaml:"compression"`
	MaxMsgBytes    int         `json:"max_msg_bytes" yaml:"max_msg_bytes"`
	Timeout        string      `json:"timeout" yaml:"timeout"`
	AckReplicas    bool        `json:"ack_replicas" yaml:"ack_replicas"`
	TargetVersion  string      `json:"target_version" yaml:"target_version"`
	TLS            btls.Config `json:"tls" yaml:"tls"`
	SASL           SASLConfig  `json:"sasl" yaml:"sasl"`
	MaxInFlight    int         `json:"max_in_flight" yaml:"max_in_flight"`
	retries.Config `json:",inline" yaml:",inline"`
	Batching       batch.PolicyConfig `json:"batching" yaml:"batching"`

	// TODO: V4 remove this.
	RoundRobinPartitions bool `json:"round_robin_partitions" yaml:"round_robin_partitions"`
}

// SASLConfig contains configuration for SASL based authentication.
type SASLConfig struct {
	Enabled  bool   `json:"enabled" yaml:"enabled"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
}

// NewKafkaConfig creates a new KafkaConfig with default values.
func NewKafkaConfig() KafkaConfig {
	rConf := retries.NewConfig()
	rConf.Backoff.InitialInterval = "0s"
	rConf.Backoff.MaxInterval = "1s"
	rConf.Backoff.MaxElapsedTime = "5s"
	batching := batch.NewPolicyConfig()
	batching.Count = 1
	return KafkaConfig{
		Addresses:            []string{"localhost:9092"},
		ClientID:             "benthos_kafka_output",
		Key:                  "",
		RoundRobinPartitions: false,
		Partitioner:          "fnv1a_hash",
		Topic:                "benthos_stream",
		Compression:          "none",
		MaxMsgBytes:          1000000,
		Timeout:              "5s",
		AckReplicas:          false,
		TargetVersion:        sarama.V1_0_0_0.String(),
		TLS:                  btls.NewConfig(),
		MaxInFlight:          1,
		Config:               rConf,
		Batching:             batching,
	}
}

//------------------------------------------------------------------------------

// Kafka is a writer type that writes messages into kafka.
type Kafka struct {
	log   log.Modular
	stats metrics.Type

	backoff backoff.BackOff

	tlsConf *tls.Config
	timeout time.Duration

	addresses []string
	version   sarama.KafkaVersion
	conf      KafkaConfig

	mDroppedMaxBytes metrics.StatCounter

	key   *text.InterpolatedBytes
	topic *text.InterpolatedString

	producer    sarama.SyncProducer
	compression sarama.CompressionCodec
	partitioner sarama.PartitionerConstructor

	connMut sync.RWMutex
}

// NewKafka creates a new Kafka writer type.
func NewKafka(conf KafkaConfig, log log.Modular, stats metrics.Type) (*Kafka, error) {
	compression, err := strToCompressionCodec(conf.Compression)
	if err != nil {
		return nil, err
	}

	// for backward compatitility
	if conf.RoundRobinPartitions {
		conf.Partitioner = "round_robin"
		log.Warnln("The field 'round_robin_partitions' is deprecated, please use the 'partitioner' field (set to 'round_robin') instead.")
	}
	partitioner, err := strToPartitioner(conf.Partitioner)
	if err != nil {
		return nil, err
	}

	k := Kafka{
		log:   log,
		stats: stats,

		conf:        conf,
		key:         text.NewInterpolatedBytes([]byte(conf.Key)),
		topic:       text.NewInterpolatedString(conf.Topic),
		compression: compression,
		partitioner: partitioner,
	}

	if tout := conf.Timeout; len(tout) > 0 {
		var err error
		if k.timeout, err = time.ParseDuration(tout); err != nil {
			return nil, fmt.Errorf("failed to parse timeout string: %v", err)
		}
	}

	if conf.TLS.Enabled {
		var err error
		if k.tlsConf, err = conf.TLS.Get(); err != nil {
			return nil, err
		}
	}

	if k.version, err = sarama.ParseKafkaVersion(conf.TargetVersion); err != nil {
		return nil, err
	}

	for _, addr := range conf.Addresses {
		for _, splitAddr := range strings.Split(addr, ",") {
			if trimmed := strings.TrimSpace(splitAddr); len(trimmed) > 0 {
				k.addresses = append(k.addresses, trimmed)
			}
		}
	}

	if k.backoff, err = conf.Config.Get(); err != nil {
		return nil, err
	}
	return &k, nil
}

//------------------------------------------------------------------------------

func strToCompressionCodec(str string) (sarama.CompressionCodec, error) {
	switch str {
	case "none":
		return sarama.CompressionNone, nil
	case "snappy":
		return sarama.CompressionSnappy, nil
	case "lz4":
		return sarama.CompressionLZ4, nil
	case "gzip":
		return sarama.CompressionGZIP, nil
	}
	return sarama.CompressionNone, fmt.Errorf("compression codec not recognised: %v", str)
}

//------------------------------------------------------------------------------

func strToPartitioner(str string) (sarama.PartitionerConstructor, error) {
	switch str {
	case "fnv1a_hash":
		return sarama.NewHashPartitioner, nil
	case "murmur2_hash":
		return sarama.NewCustomPartitioner(
			sarama.WithAbsFirst(),
			sarama.WithCustomHashFunction(murmur2.New32),
		), nil
	case "random":
		return sarama.NewRandomPartitioner, nil
	case "round_robin":
		return sarama.NewRoundRobinPartitioner, nil
	default:
	}
	return nil, fmt.Errorf("partitioner not recognised: %v", str)
}

//------------------------------------------------------------------------------

func buildHeaders(version sarama.KafkaVersion, part types.Part) []sarama.RecordHeader {
	if version.IsAtLeast(sarama.V0_11_0_0) {
		out := []sarama.RecordHeader{}
		meta := part.Metadata()
		meta.Iter(func(k, v string) error {
			out = append(out, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
			return nil
		})
		return out
	}

	// no headers before version 0.11
	return nil
}

//------------------------------------------------------------------------------

// ConnectWithContext attempts to establish a connection to a Kafka broker.
func (k *Kafka) ConnectWithContext(ctx context.Context) error {
	return k.Connect()
}

// Connect attempts to establish a connection to a Kafka broker.
func (k *Kafka) Connect() error {
	k.connMut.Lock()
	defer k.connMut.Unlock()

	if k.producer != nil {
		return nil
	}

	config := sarama.NewConfig()
	config.ClientID = k.conf.ClientID

	config.Version = k.version

	config.Producer.Compression = k.compression
	config.Producer.Partitioner = k.partitioner
	config.Producer.MaxMessageBytes = k.conf.MaxMsgBytes
	config.Producer.Timeout = k.timeout
	config.Producer.Return.Errors = true
	config.Producer.Return.Successes = true
	config.Net.TLS.Enable = k.conf.TLS.Enabled
	if k.conf.TLS.Enabled {
		config.Net.TLS.Config = k.tlsConf
	}
	if k.conf.SASL.Enabled {
		config.Net.SASL.Enable = true
		config.Net.SASL.User = k.conf.SASL.User
		config.Net.SASL.Password = k.conf.SASL.Password
	}

	if k.conf.AckReplicas {
		config.Producer.RequiredAcks = sarama.WaitForAll
	} else {
		config.Producer.RequiredAcks = sarama.WaitForLocal
	}

	var err error
	k.producer, err = sarama.NewSyncProducer(k.addresses, config)

	if err == nil {
		k.log.Infof("Sending Kafka messages to addresses: %s\n", k.addresses)
	}
	return err
}

// WriteWithContext will attempt to write a message to Kafka, wait for
// acknowledgement, and returns an error if applicable.
func (k *Kafka) WriteWithContext(ctx context.Context, msg types.Message) error {
	return k.Write(msg)
}

// Write will attempt to write a message to Kafka, wait for acknowledgement, and
// returns an error if applicable.
func (k *Kafka) Write(msg types.Message) error {
	k.connMut.RLock()
	producer := k.producer
	version := k.version
	k.connMut.RUnlock()

	if producer == nil {
		return types.ErrNotConnected
	}

	msgs := []*sarama.ProducerMessage{}
	msg.Iter(func(i int, p types.Part) error {
		lMsg := message.Lock(msg, i)

		key := k.key.Get(lMsg)
		nextMsg := &sarama.ProducerMessage{
			Topic:   k.topic.Get(lMsg),
			Value:   sarama.ByteEncoder(p.Get()),
			Headers: buildHeaders(version, p),
		}
		if len(key) > 0 {
			nextMsg.Key = sarama.ByteEncoder(key)
		}
		msgs = append(msgs, nextMsg)
		return nil
	})

	err := producer.SendMessages(msgs)
	for err != nil {
		pErrs, ok := err.(sarama.ProducerErrors)
		if !ok {
			k.log.Errorf("Failed to send messages: %v\n", err)
		} else {
			if len(pErrs) == 0 {
				break
			}
			msgs = nil
			for _, pErr := range pErrs {
				msgs = append(msgs, pErr.Msg)
			}
			k.log.Errorf("Failed to send '%v' messages: %v\n", len(pErrs), pErrs[0].Err)
		}

		tNext := k.backoff.NextBackOff()
		if tNext == backoff.Stop {
			k.backoff.Reset()
			return err
		}
		<-time.After(tNext)

		// Recheck connection is alive
		k.connMut.RLock()
		producer = k.producer
		k.connMut.RUnlock()

		if producer == nil {
			return types.ErrNotConnected
		}
		err = producer.SendMessages(msgs)
	}

	k.backoff.Reset()
	return nil
}

// CloseAsync shuts down the Kafka writer and stops processing messages.
func (k *Kafka) CloseAsync() {
	go func() {
		k.connMut.Lock()
		if nil != k.producer {
			k.producer.Close()
			k.producer = nil
		}
		k.connMut.Unlock()
	}()
}

// WaitForClose blocks until the Kafka writer has closed down.
func (k *Kafka) WaitForClose(timeout time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
