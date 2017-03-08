package beater

import (
	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/jahor/rmqbeat/config"
)

type rmqbeat struct {
	done      chan struct{}
	config    config.Config
	name      string
	version   string
	consumers []*Consumer
}

// New creates a rmqbeater from configuration
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &rmqbeat{
		done:    make(chan struct{}),
		config:  config,
		name:    b.Name,
		version: b.Version,
	}
	return bt, nil
}

// Run starts rmqbeater
func (bt *rmqbeat) Run(b *beat.Beat) error {
	logp.Info("rmqbeat is running! Hit CTRL-C to stop it.")

	bt.consumers = make([]*Consumer, len(bt.config.Consumers))
	for i, consumerConfig := range bt.config.Consumers {
		cons := NewConsumer(consumerConfig, b.Publisher.Connect(), bt.name, bt.version)
		bt.consumers[i] = cons
	}

	for _, cons := range bt.consumers {
		cons.Connect()
	}

	logp.Info("running forever")
	select {
	case <-bt.done:
		break
	}

	logp.Info("shutting down")

	for _, c := range bt.consumers {
		if err := c.Shutdown(); err != nil {
			logp.Err("error during shutdown: %s", c, err)
		}
	}
	return nil
}

// Stop cancels all consumers
func (bt *rmqbeat) Stop() {
	close(bt.done)
}
