package output

import (
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/output/writer"
	"github.com/Jeffail/benthos/v3/lib/types"
)

//------------------------------------------------------------------------------

func init() {
	Constructors[TypeSNS] = TypeSpec{
		constructor: NewAmazonSNS,
		Description: `
Sends messages to an AWS SNS topic.

### Credentials

By default Benthos will use a shared credentials file when connecting to AWS
services. It's also possible to set them explicitly at the component level,
allowing you to transfer data across accounts. You can find out more
[in this document](../aws.md).`,
		Async: true,
	}
}

//------------------------------------------------------------------------------

// NewAmazonSNS creates a new AmazonSNS output type.
func NewAmazonSNS(conf Config, mgr types.Manager, log log.Modular, stats metrics.Type) (Type, error) {
	s, err := writer.NewSNS(conf.SNS, log, stats)
	if err != nil {
		return nil, err
	}
	if conf.SNS.MaxInFlight == 1 {
		return NewWriter(
			TypeSNS, s, log, stats,
		)
	}
	return NewAsyncWriter(
		TypeSNS, conf.SNS.MaxInFlight, s, log, stats,
	)
}

//------------------------------------------------------------------------------
