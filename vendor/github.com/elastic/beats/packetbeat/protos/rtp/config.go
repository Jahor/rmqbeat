package rtp

import (
	"github.com/elastic/beats/packetbeat/config"
	"github.com/elastic/beats/packetbeat/protos"
)

type rtpConfig struct {
	config.ProtocolCommon `config:",inline"`
}

var (
	defaultConfig = rtpConfig{
		ProtocolCommon: config.ProtocolCommon{
			TransactionTimeout: protos.DefaultTransactionExpiration,
		},
	}
)
