package output

import (
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/output/writer"
	"github.com/Jeffail/benthos/v3/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeRedisStreams] = TypeSpec{
		constructor: NewRedisStreams,
		Description: `
Pushes messages to a Redis (v5.0+) Stream (which is created if it doesn't
already exist) using the XADD command. It's possible to specify a maximum length
of the target stream by setting it to a value greater than 0, in which case this
cap is applied only when Redis is able to remove a whole macro node, for
efficiency.

Redis stream entries are key/value pairs, as such it is necessary to specify the
key to be set to the body of the message. All metadata fields of the message
will also be set as key/value pairs, if there is a key collision between
a metadata item and the body then the body takes precedence.`,
		Async: true,
	}
}

//------------------------------------------------------------------------------

// NewRedisStreams creates a new RedisStreams output type.
func NewRedisStreams(conf Config, mgr types.Manager, log log.Modular, stats metrics.Type) (Type, error) {
	w, err := writer.NewRedisStreams(conf.RedisStreams, log, stats)
	if err != nil {
		return nil, err
	}
	if conf.RedisStreams.MaxInFlight == 1 {
		return NewWriter(TypeRedisStreams, w, log, stats)
	}
	return NewAsyncWriter(TypeRedisStreams, conf.RedisStreams.MaxInFlight, w, log, stats)
}

//------------------------------------------------------------------------------
