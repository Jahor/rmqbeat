// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

import (
	"time"
	"github.com/elastic/beats/libbeat/outputs"
	"github.com/elastic/beats/libbeat/common"
)

type Config struct {
	Consumers []*common.Config `config:"consumers"`
}

type QueueConfig struct {
	// The name of the queue rmqbeat will consume events from. If
	// left empty, a transient queue with an randomly chosen name
	// will be created.
	Name       string `config:"name"`

	// Is this queue durable? (aka; Should it survive a broker restart?)
	//
	// Default: true
	Durable    bool `config:"durable"`

	// Should the queue be deleted on the broker when the last consumer
	// disconnects? Set this option to `false` if you want the queue to remain
	// on the broker, queueing up messages until a consumer comes along to
	// consume them.
	//
	// Default: false
	AutoDelete bool `config:"auto_delete"`


	// Is the queue exclusive? Exclusive queues can only be used by the connection
	// that declared them and will be deleted when it is closed (e.g. due to a Logstash
	// restart).
	//
	// Default: false
	Exclusive  bool `config:"exclusive"`

	// If true the queue will be passively declared, meaning it must
	// already exist on the server. To have Logstash create the queue
	// if necessary leave this option as false. If actively declaring
	// a queue that already exists, the queue options for this plugin
	// (durable etc) must match those of the existing queue.
	//
	// Default: false
	Passive    bool `config:"passive"`

	// Extra queue arguments as an array.
	// To make a RabbitMQ queue mirrored, use: `{"x-ha-policy" => "all"}`
	//
	// Default: {}
	Arguments  map[string]interface{} `config:"arguments"`
}

type ExchangeConfig struct {
	// The name of the exchange rmqbeat will consume events from. If
	// left empty, a transient queue with an randomly chosen name
	// will be created.
	Name       string `config:"name"`

	// The type of the exchange to bind to. Specifying this will cause this plugin
	// to declare the exchange if it does not exist.
	Type       string `config:"type"`

	// Is this exchange durable? (aka; Should it survive a broker restart?)
	//
	// Default: true
	Durable    bool `config:"durable"`

	// Should the exchange be deleted on the broker when the last consumer
	// disconnects? Set this option to `false` if you want the queue to remain
	// on the broker, queueing up messages until a consumer comes along to
	// consume them.
	//
	// Default: false
	AutoDelete bool `config:"auto_delete"`

	// Should the exchange be created internal.
	//
	// Default: false
	Internal   bool `config:"internal"`

	// If true the exchange will be passively declared, meaning it must
	// already exist on the server. To have Logstash create the queue
	// if necessary leave this option as false. If actively declaring
	// a queue that already exists, the queue options for this plugin
	// (durable etc) must match those of the existing queue.
	//
	// Default: false
	Passive    bool `config:"passive"`

	// Extra exchange arguments as an array.
	//
	// Default: {}
	Arguments  map[string]interface{} `config:"arguments"`
}

type ConnectionConfig struct {
	// RabbitMQ server address(es)
	// host can either be a single host, or a list of hosts
	// i.e.
	//   host: "localhost"
	// or
	//   host: ["host01", "host02]
	//
	// if multiple hosts are provided on the initial connection and any subsequent
	// recovery attempts of the hosts is chosen at random and connected to.
	// Note that only one host connection is active at a time.
	//
	// Default: [localhost]
	Host                 []string `config:"host"`

	// RabbitMQ port to connect on
	//
	// Default: 5672
	Port                 uint16 `config:"port"`

	// The vhost (virtual host) to use. If you don't know what this
	// is, leave the default. With the exception of the default
	// vhost ("/"), names of vhosts should not begin with a forward
	// slash.
	//
	// Default: /
	Vhost                string `config:"vhost"`

	// RabbitMQ username
	//
	// Default: guest
	User                 string `config:"user"`

	// RabbitMQ password
	//
	// Default: guest
	Password             string `config:"password"`


	// Set this to automatically recover from a broken connection. You almost certainly don't want to override this!!!
	//
	// Default: true
	AutomaticRecovery    bool `config:"automatic_recovery"`

	// Time in seconds to wait before retrying a connection
	//
	// Default: 5s
	ConnectRetryInterval time.Duration `config:"connect_retry_interval"`

	// The default connection timeout in milliseconds. If not specified the timeout is infinite.
	//
	// Default: 20s
	ConnectionTimeout    time.Duration `config:"connection_timeout"`

	// Heartbeat delay in seconds. If unspecified no heartbeats will be sent
	//
	// Default: 25s
	Heartbeat            time.Duration `config:"heartbeat"`

	// TLS configuration
	TLS                  *outputs.TLSConfig `config:"ssl"`

	// Name of connection that RabbitMQ displays in Management console
	//
	// Default: rmqbeat
	Name                 string `config:"name"`
}

type ConsumerConfig struct {
	DocumentType     string `config:"document_type"`


	// Connection parameters
	Connection       ConnectionConfig `config:"connection"`

	// The name of the exchange to bind the queue to. Specify `exchange_type`
	// as well to declare the exchange if it does not exist
	Exchange         ExchangeConfig `config:"exchange"`

	// The routing key to use when binding a queue to the exchange.
	// This is only relevant for direct or topic exchanges.
	//
	// * Routing keys are ignored on fanout exchanges.
	// * Wildcards are not valid on direct exchanges.
	RoutingKey       string `config:"routing_key"`

	// Extra queue arguments as an array.
	// To make a RabbitMQ queue mirrored, use: `{"x-ha-policy" => "all"}`
	RoutingArguments map[string]interface{} `config:"routing_arguments"`

	// The queue rmqbeat will consume events from. If
	// left empty, a transient queue with an randomly chosen name
	// will be created.
	Queue            QueueConfig `config:"queue"`

	// Prefetch count. If acknowledgements are enabled with the `ack`
	// option, specifies the number of outstanding unacknowledged
	// messages allowed.
	//
	// Default: 256
	PrefetchCount    int `config:"prefetch_count"`

	// Enable message acknowledgements. With acknowledgements
	// messages fetched by Logstash but not yet sent into the
	// Logstash pipeline will be requeued by the server if Logstash
	// shuts down. Acknowledgements will however hurt the message
	// throughput.
	// This will only send an ack back every `prefetch_count` messages.
	// Working in batches provides a performance boost here.
	//
	// Default: true
	Ack              bool `config:"ack"`

	// Mode for reading messages produced by RabbitMQ tracer published on
	// amq.rabbitmq.trace exchange.
	// Event produced will look like if it was received directly
	// And adding some more information like user, connection, channel
	// When true automatically populates exchange and routing key
	//
	// Default: false
	TracerMode       bool `config:"tracer"`

	common.EventMetadata `config:",inline"` // Fields and tags to add to events.
}

var DefaultConfig = Config{
	Consumers: []*common.Config{},
}

var DefaultConnectionConfig = ConnectionConfig{
	Host: []string{"localhost"},
	Vhost: "/",
	Port: 5672,
	User: "guest",
	Password: "guest",
	AutomaticRecovery: true,
	ConnectRetryInterval: 5 * time.Second,
	ConnectionTimeout: 20 * time.Second,
	Heartbeat: 25 * time.Second,
	TLS: nil,
	Name: "rmqbeat",
}

var DefaultConsumerConfig = ConsumerConfig{
	DocumentType: "rmq_message",
	PrefetchCount: 256,
	Connection: DefaultConnectionConfig,
	Ack: true,
	TracerMode:false,
	Exchange: DefaultExchangeConfig,
	Queue: DefaultQueueConfig,
}

var DefaultTracerConsumerConfig = ConsumerConfig{
	DocumentType: "rmq_message",
	PrefetchCount: 256,
	Connection: DefaultConnectionConfig,
	Ack: true,
	TracerMode:false,
	Exchange: DefaultTraceExchangeConfig,
	Queue: DefaultQueueConfig,
	RoutingKey: "#",
}

var DefaultTraceExchangeConfig = ExchangeConfig{
	Name: "amq.rabbitmq.trace",
	Durable: true,
	AutoDelete: false,
	Internal: true,
	Passive: false,
}

var DefaultExchangeConfig = ExchangeConfig{
	Durable: true,
	AutoDelete: false,
	Internal: false,
	Passive: false,
}

var DefaultQueueConfig = QueueConfig{
	Durable: true,
	AutoDelete: false,
	Passive: false,
}


