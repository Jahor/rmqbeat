package beater

import (
	"fmt"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/jahor/rmqbeat/config"
)

type Rmqbeat struct {
	done      chan struct{}
	config    config.Config
	consumers []*Consumer
}

// Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &Rmqbeat{
		done:   make(chan struct{}),
		config: config,
	}
	return bt, nil
}

func (bt *Rmqbeat) Run(b *beat.Beat) error {
	logp.Info("rmqbeat is running! Hit CTRL-C to stop it.")

	bt.consumers = make([]*Consumer, len(bt.config.Consumers))
	for i, consumerConfig := range bt.config.Consumers {
		cons := NewConsumer(consumerConfig, b.Publisher.Connect())
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

func (bt *Rmqbeat) Stop() {
	close(bt.done)
}
