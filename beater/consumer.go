package beater

import (
	"crypto/tls"
	"fmt"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/outputs"
	"github.com/elastic/beats/libbeat/publisher"
	"github.com/jahor/rmqbeat/config"
	"github.com/streadway/amqp"
	"golang.org/x/text/encoding/unicode"
	"net"
	"strings"
	"time"
)

// Consumer is a basic AMQP message receiver, that publishes received message to configured outputs
type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	name    string
	version string
	tag     string
	done    chan error
	config  config.ConsumerConfig
	hostIdx int
	client  publisher.Client
	maker   func(d amqp.Delivery) *common.MapStr
}

func buildURL(connectionConfig *config.ConnectionConfig, hostIdx int) (string, *tls.Config, error) {
	host := connectionConfig.Host[hostIdx]
	var schema string
	var ssl *tls.Config
	if connectionConfig.TLS.IsEnabled() {
		connectionConfig.TLS.Validate()
		schema = "amqps"

		cfg, err := outputs.LoadTLSConfig(connectionConfig.TLS)

		if err != nil {
			return "", nil, err
		}

		ssl = cfg.BuildModuleConfig(host)
	} else {
		schema = "amqp"
		ssl = nil
	}
	var auth string
	if connectionConfig.User != "" {
		auth = connectionConfig.User + ":" + connectionConfig.Password + "@"
	} else {
		auth = ""
	}

	return fmt.Sprintf("%s://%s%s:%d", schema, auth, host, connectionConfig.Port), ssl, nil
}

// Connect starts connection to AMQP
func (c *Consumer) Connect() {
	err := c.connect()
	if err != nil {
		logp.Info("Error connecting: %q", err)
		go c.reconnect()
	}
}

func (c *Consumer) reconnect() {
	if c.config.Connection.AutomaticRecovery {
		for {
			c.channel = nil
			c.conn = nil

			ticker := time.NewTicker(c.config.Connection.ConnectRetryInterval)

			logp.Info("Waiting to reconnect...")

			select {
			case <-c.done:
				logp.Info("Cancelled while waiting to reconnect")
				return
			case <-ticker.C:
				logp.Info("Reconnecting...")
				err := c.connect()
				if err == nil {
					ticker.Stop()
					return
				}
				logp.Info("Error connecting: ", err)
			}

			ticker.Stop()
		}
	}
}

func (c *Consumer) connect() error {
	config := c.config
	connectionConfig := &config.Connection
	var err error

	amqpURI, ssl, err := buildURL(connectionConfig, c.hostIdx)
	c.hostIdx = (c.hostIdx + 1) % len(connectionConfig.Host)
	if err != nil {
		return fmt.Errorf("TLS Config Load: %s", err)
	}

	logp.Info("[%s] dialing %q", config.Connection.Name, amqpURI)
	c.conn, err = amqp.DialConfig(amqpURI, amqp.Config{
		Heartbeat:       connectionConfig.Heartbeat,
		Vhost:           connectionConfig.Vhost,
		TLSClientConfig: ssl,
		Properties: amqp.Table{
			"product":         c.name,
			"version":         c.version,
			"connection_name": connectionConfig.Name,
		},
		Dial: func(network, addr string) (net.Conn, error) {
			conn, err := net.DialTimeout(network, addr, connectionConfig.ConnectionTimeout)
			if err != nil {
				return nil, err
			}

			// Heartbeating hasn't started yet, don't stall forever on a dead server.
			// A deadline is set for TLS and AMQP handshaking. After AMQP is established,
			// the deadline is cleared in openComplete.
			if err := conn.SetDeadline(time.Now().Add(connectionConfig.ConnectionTimeout)); err != nil {
				return nil, err
			}

			return conn, nil
		},
	})
	if err != nil {
		return fmt.Errorf("Dial: %s", err)
	}

	go func() {
		logp.Warn("closing: %s", <-c.conn.NotifyClose(make(chan *amqp.Error)))

		// Wait for channel to stop
		<-c.done

		c.reconnect()
	}()

	logp.Info("[%s] got Connection, getting Channel", config.Connection.Name)
	c.channel, err = c.conn.Channel()
	if err != nil {
		return fmt.Errorf("Channel: %s", err)
	}

	if config.Exchange.Type != "" {
		logp.Info("[%s] got Channel, declaring Exchange (%q)", config.Connection.Name, config.Exchange.Name)
		if err = c.channel.ExchangeDeclare(
			config.Exchange.Name,       // name of the exchange
			config.Exchange.Type,       // type
			config.Exchange.Durable,    // durable
			config.Exchange.AutoDelete, // delete when complete
			config.Exchange.Internal,   // internal
			config.Exchange.Passive,    // noWait
			config.Exchange.Arguments,  // arguments
		); err != nil {
			return fmt.Errorf("Exchange Declare: %s", err)
		}
	}

	logp.Info("[%s] declared Exchange, declaring Queue %q", config.Connection.Name, config.Queue.Name)
	queue, err := c.channel.QueueDeclare(
		config.Queue.Name,       // name of the queue
		config.Queue.Durable,    // durable
		config.Queue.AutoDelete, // delete when unused
		config.Queue.Exclusive,  // exclusive
		config.Queue.Passive,    // noWait
		config.Queue.Arguments,  // arguments
	)

	if err != nil {
		return fmt.Errorf("Queue Declare: %s", err)
	}

	logp.Info("[%s] declared Queue (%q %d messages, %d consumers), binding to Exchange %q (key %q)", config.Connection.Name,
		queue.Name, queue.Messages, queue.Consumers, config.Exchange.Name, config.RoutingKey)

	if err = c.channel.QueueBind(
		queue.Name,           // name of the queue
		config.RoutingKey,    // bindingKey
		config.Exchange.Name, // sourceExchange
		false,                // noWait
		config.RoutingArguments, // arguments
	); err != nil {
		return fmt.Errorf("Queue Bind: %s", err)
	}

	logp.Info("[%s] Queue bound to Exchange, starting Consume (consumer tag %q)", config.Connection.Name, c.tag)
	deliveries, err := c.channel.Consume(
		queue.Name,  // name
		c.tag,       // consumerTag,
		!config.Ack, // noAck
		false,       // exclusive
		false,       // noLocal
		false,       // noWait
		nil,         // arguments
	)
	if err != nil {
		return fmt.Errorf("Queue Consume: %s", err)
	}

	go handle(c, deliveries, c.done)

	return nil
}

// NewConsumer creates a consumer from configuration
func NewConsumer(cfg *common.Config, client publisher.Client, name string, version string) *Consumer {

	var consumerConfig config.ConsumerConfig

	tracer, err := cfg.Bool("tracer", -1)

	if err == nil && tracer {
		logp.Info("Configuring tracer")
		consumerConfig = config.DefaultTracerConsumerConfig
	} else {
		consumerConfig = config.DefaultConsumerConfig
	}

	if err := cfg.Unpack(&consumerConfig); err != nil {
		return nil
	}

	logp.Info("[%s] Configuring consumer", consumerConfig.Connection.Name)
	logp.Info("[%s] Exchange: %s", consumerConfig.Connection.Name, consumerConfig.Exchange.Name)
	logp.Info("[%s] Routing Key: %s", consumerConfig.Connection.Name, consumerConfig.RoutingKey)
	logp.Info("[%s] Queue: %s", consumerConfig.Connection.Name, consumerConfig.Queue.Name)

	var maker func(d amqp.Delivery) *common.MapStr
	if consumerConfig.TracerMode {
		maker = func(d amqp.Delivery) *common.MapStr {
			return makeTraceEvent(d, consumerConfig.DocumentType)
		}
	} else {
		maker = func(d amqp.Delivery) *common.MapStr {
			return makeEvent(d, consumerConfig.DocumentType, consumerConfig.Queue.Name)
		}
	}
	c := &Consumer{
		conn:    nil,
		channel: nil,
		tag:     fmt.Sprintf("%s--%s--%s", consumerConfig.Exchange.Name, consumerConfig.RoutingKey, consumerConfig.Queue.Name),
		done:    make(chan error),
		client:  client,
		maker:   maker,
		config:  consumerConfig,
		name:    name,
		version: version,
	}

	return c
}

// Shutdown cancels consumer and closes connection
func (c *Consumer) Shutdown() error {

	// If we are not waiting to reconnect
	if c.channel != nil {
		// will close() the deliveries channel
		if err := c.channel.Cancel(c.tag, true); err != nil {
			return fmt.Errorf("Consumer cancel failed: %s", err)
		}

		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("AMQP connection close error: %s", err)
		}

		defer logp.Info("AMQP shutdown OK")

		c.client.Close()

		// wait for handle() to exit
		return <-c.done
	}

	logp.Info("Stopping in reconnect")
	// Cancel wait to reconnect
	c.done <- nil
	return nil
}

func handle(consumer *Consumer, deliveries <-chan amqp.Delivery, done chan error) {
	for d := range deliveries {
		logp.Info(
			"got %dB delivery: [%v] %q",
			len(d.Body),
			d.DeliveryTag,
			d.Body,
		)
		event := consumer.maker(d)
		if event != nil {

			common.AddTags(*event, consumer.config.EventMetadata.Tags)
			common.MergeFields(*event, consumer.config.EventMetadata.Fields, consumer.config.EventMetadata.FieldsUnderRoot)

			consumer.client.PublishEvent(*event)
		}
		logp.Info("Event sent")
		if consumer.config.Ack {
			d.Ack(false)
		}
	}
	logp.Info("handle: deliveries channel closed")
	done <- nil
}

func cleanMap(m map[string]interface{}) map[string]interface{} {
	r := make(map[string]interface{})
	for k := range m {
		v := m[k]
		if v != nil && v != "" {
			r[k] = v
		}
	}
	return r
}

// TODO add proper decoding of custom encodings
func decode(body []byte, contentEncoding string) (string, error) {
	// utf8_body := make([]byte, len(body))
	enc := unicode.UTF8
	str, err := enc.NewDecoder().String(string(body))
	if err != nil {
		logp.Err("Can not decode %s text '%s': %s", contentEncoding, body, err)
		return str, err
	}
	return str, nil
}

func nullify(s string) *string {
	if len(s) == 0 {
		return nil
	}
	return &s
}

func nullifyInt(s int) *int {
	if s == 0 {
		return nil
	}
	return &s
}

func nullifyTime(s time.Time) *time.Time {
	if s.IsZero() {
		return nil
	}
	return &s
}

func makeEvent(d amqp.Delivery, documentType string, queue string) *common.MapStr {
	payloadBody, _ := decode(d.Body, d.ContentEncoding)
	var timestamp common.Time
	if d.Timestamp.IsZero() {
		timestamp = common.Time(time.Now())
	} else {
		timestamp = common.Time(d.Timestamp)
	}

	event := &common.MapStr{
		"@timestamp": timestamp,
		"type":       documentType,
		"rabbitmq": RabbitMQEvent{
			Properties: AMQPProperties{
				ContentType:     nullify(d.ContentType),
				ContentEncoding: nullify(d.ContentEncoding),
				DeliveryMode:    nullifyInt(int(d.DeliveryMode)),
				Priority:        nullifyInt(int(d.Priority)),
				CorrelationID:   nullify(d.CorrelationId),
				ReplyTo:         nullify(d.ReplyTo),
				MessageID:       nullify(d.MessageId),
				Timestamp:       nullifyTime(d.Timestamp),
				Type:            nullify(d.Type),
				UserID:          nullify(d.UserId),
				AppID:           nullify(d.AppId),
				Expiration:      nullify(d.Expiration),
			},
			Headers:     cleanMap(d.Headers),
			Action:      "receive",
			Queue:       nullify(queue),
			ConsumerTag: nullify(d.ConsumerTag),
			Exchange:    d.Exchange,
			Redelivered: d.Redelivered,
			RoutingKey:  d.RoutingKey,
			Payload: Payload{
				Size: len(d.Body),
				Body: payloadBody,
			},
		},
	}
	return event
}

func optionalString(something interface{}) *string {
	if something == nil {
		return nil
	}
	str := something.(string)
	return &str
}

func optionalInt(something interface{}) *int {
	if something == nil {
		return nil
	}
	num := int(something.(int32))
	return &num
}

func optionalStringArray(something interface{}) *[]string {
	if something == nil {
		return nil
	}
	l := something.([]interface{})
	result := make([]string, len(l))
	for i, s := range l {
		result[i] = s.(string)
	}

	return &result
}

func optionalBool(something interface{}) bool {
	if something == nil {
		return false
	}
	return something.(bool)
}

func optionalTable(something interface{}) amqp.Table {
	if something == nil {
		return amqp.Table{}
	}
	return something.(amqp.Table)
}

func makeTraceEvent(d amqp.Delivery, documentType string) *common.MapStr {
	if d.Headers["exchange_name"] == "amq.rabbitmq.log" {
		return nil
	}
	realProperties := optionalTable(d.Headers["properties"])
	realHeaders := optionalTable(realProperties["headers"])

	logp.Info("RK: %s", d.RoutingKey)

	actionSubject := strings.SplitN(d.RoutingKey, ".", 2)
	action := actionSubject[0]
	subject := actionSubject[1]
	var queue string
	if action == "deliver" {
		queue = subject
	} else {
		queue = ""
	}

	payloadBody, _ := decode(d.Body, d.ContentEncoding)

	var realTimestamp *time.Time
	mayBeRealTimestamp, hasRealTimestamp := realProperties["timestamp"]
	var timestamp common.Time
	if hasRealTimestamp {
		ts := time.Unix(int64(mayBeRealTimestamp.(int32)), 0)
		timestamp = common.Time(ts)
		realTimestamp = &ts
	} else {
		if d.Timestamp.IsZero() {
			timestamp = common.Time(time.Now())
		} else {
			timestamp = common.Time(d.Timestamp)
		}
	}

	event := &common.MapStr{
		"@timestamp": timestamp,
		"type":       documentType,
		"rabbitmq": RabbitMQEvent{
			Properties: AMQPProperties{
				ContentType:     optionalString(realProperties["content_type"]),
				ContentEncoding: optionalString(realProperties["content_encoding"]),
				DeliveryMode:    optionalInt(realProperties["delivery_mode"]),
				Priority:        optionalInt(realProperties["priority"]),
				CorrelationID:   optionalString(realProperties["correlation_id"]),
				ReplyTo:         optionalString(realProperties["reply_to"]),
				MessageID:       optionalString(realProperties["message_id"]),
				Timestamp:       realTimestamp,
				Type:            optionalString(realProperties["type"]),
				UserID:          optionalString(realProperties["user_id"]),
				AppID:           optionalString(realProperties["app_id"]),
				Expiration:      optionalString(realProperties["expiration"]),
			},
			Headers:      cleanMap(realHeaders),
			Action:       action,
			Queue:        nullify(queue),
			Connection:   optionalString(d.Headers["connection"]),
			Channel:      optionalInt(d.Headers["channel"]),
			User:         optionalString(d.Headers["user"]),
			RoutedQueues: optionalStringArray(d.Headers["routed_queues"]),
			Exchange:     d.Headers["exchange_name"].(string),
			Redelivered:  optionalBool(d.Headers["redelivered"]),
			RoutingKey:   (*optionalStringArray(d.Headers["routing_keys"]))[0],
			Payload: Payload{
				Size: len(d.Body),
				Body: payloadBody,
			},
		},
	}

	return event
}
