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
	client    beat.Client
	consumers []*Consumer
	log       *logp.Logger
}

// Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	c := config.DefaultConfig
	if err := cfg.Unpack(&c); err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	bt := &Rmqbeat{
		done:   make(chan struct{}),
		config: c,
		log:    logp.NewLogger("rmqbeat"),
	}
	return bt, nil
}

func (bt *Rmqbeat) Run(b *beat.Beat) error {
	logp.Info("rmqbeat is running! Hit CTRL-C to stop it.")

	var err error
	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}

	bt.consumers = make([]*Consumer, len(bt.config.Consumers))
	for i, consumerConfig := range bt.config.Consumers {
		client, err := b.Publisher.Connect()
		if err != nil {
			return fmt.Errorf("error creating consumer: %v", err)
		}
		cons := NewConsumer(consumerConfig, client, b.Info.Name, b.Info.Version)
		bt.consumers[i] = cons
	}

	for _, cons := range bt.consumers {
		cons.Connect()
	}

	bt.log.Info("running forever")
	select {
	case <-bt.done:
		break
	}

	bt.log.Info("shutting down")

	for _, c := range bt.consumers {
		bt.log.Infof("Shutting down %s...", c.config.Connection.Name)
		if err := c.Shutdown(); err != nil {
			bt.log.Errorf("error during shutdown: %s", c.config.Connection.Name, err)
		} else {
			bt.log.Infof("%s Shut down", c.config.Connection.Name)
		}
	}
	return nil

	//ticker := time.NewTicker(bt.config.Period)
	// counter := 1
	// for {
	// 	select {
	// 	case <-bt.done:
	// 		return nil
	// 	case <-// ticker.C:
	// 	}
	//
	// 	event := beat.Event{
	// 		Timestamp: time.Now(),
	// 		Fields: common.MapStr{
	// 			"type":    b.Info.Name,
	// 			"counter": counter,
	// 		},
	// 	}
	// 	bt.client.Publish(event)
	// 	logp.Info("Event sent")
	// 	counter++
	// }
}

func (bt *Rmqbeat) Stop() {
	bt.client.Close()
	close(bt.done)
}
