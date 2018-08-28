package beater

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/outputs"
	"github.com/jahor/rmqbeat/config"
	"github.com/streadway/amqp"
	"golang.org/x/text/encoding/unicode"
	"net"
	"strconv"
	"strings"
	"time"
	"crypto/sha256"
	"encoding/hex"
)

const (
	none      = ""
	evo_trace = "evo_trace"
	log       = "log"
	event     = "event"
	trace     = "trace"
)

// Consumer is a basic AMQP message receiver, that publishes received message to configured outputs
type Consumer struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	name     string
	version  string
	tag      string
	done     chan error
	config   config.ConsumerConfig
	hostIdx  int
	client   beat.Client
	log      *logp.Logger
	maker    func(c *Consumer, d amqp.Delivery) *beat.Event
	shutdown bool
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
		c.log.Errorf("Error connecting: %v", err)
		go c.reconnect()
	}
}

func (c *Consumer) reconnect() {
	if c.config.Connection.AutomaticRecovery {
		for {
			c.log.Info("Reconnecting...")
			if c.channel != nil {
				c.channel.Close()
				c.channel = nil
			}
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
			}

			ticker := time.NewTicker(c.config.Connection.ConnectRetryInterval)

			c.log.Info("Waiting to reconnect...")

			select {
			case <-c.done:
				c.log.Info("Cancelled while waiting to reconnect")
				return
			case <-ticker.C:
				c.log.Info("Reconnecting...")
				err := c.connect()
				if err == nil {
					ticker.Stop()
					return
				}
				c.log.Errorf("Error connecting: %v", err)
			}

			ticker.Stop()
		}
	}
}

func (c *Consumer) connect() error {
	cfg := c.config
	connectionConfig := &cfg.Connection
	var err error

	amqpURI, ssl, err := buildURL(connectionConfig, c.hostIdx)
	c.hostIdx = (c.hostIdx + 1) % len(connectionConfig.Host)
	if err != nil {
		return fmt.Errorf("TLS Config Load: %s", err)
	}

	c.log.Infof("dialing %q", amqpURI)
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
		return fmt.Errorf("dial: %s", err)
	}

	go func() {
		var closeErr *amqp.Error
		closeErr = <-c.conn.NotifyClose(make(chan *amqp.Error))
		if closeErr != nil {
			c.log.Warnf("closing: %v", err)

			// Wait for channel to stop
			<-c.done

			c.reconnect()
		} else {
			c.log.Info("closing")
		}

	}()

	c.log.Info("got Connection, getting Channel")
	c.channel, err = c.conn.Channel()
	if err != nil {
		return fmt.Errorf("channel: %s", err)
	}

	if cfg.Exchange.Type != "" {
		c.log.Infof("got Channel, declaring Exchange (%q)", cfg.Exchange.Name)
		if err = c.channel.ExchangeDeclare(
			cfg.Exchange.Name,       // name of the exchange
			cfg.Exchange.Type,       // type
			cfg.Exchange.Durable,    // durable
			cfg.Exchange.AutoDelete, // delete when complete
			cfg.Exchange.Internal,   // internal
			cfg.Exchange.Passive,    // noWait
			cfg.Exchange.Arguments,  // arguments
		); err != nil {
			return fmt.Errorf("exchange Declare: %s", err)
		}
	}

	c.log.Infof("declared Exchange, declaring Queue %q", cfg.Queue.Name)
	queue, err := c.channel.QueueDeclare(
		cfg.Queue.Name,       // name of the queue
		cfg.Queue.Durable,    // durable
		cfg.Queue.AutoDelete, // delete when unused
		cfg.Queue.Exclusive,  // exclusive
		cfg.Queue.Passive,    // noWait
		cfg.Queue.Arguments,  // arguments
	)

	if err != nil {
		return fmt.Errorf("queue Declare: %s", err)
	}

	if cfg.Exchange.Name != "" {
		c.log.Infof("declared Queue (%q %d messages, %d consumers), binding to Exchange %q (key %q)",
			queue.Name, queue.Messages, queue.Consumers, cfg.Exchange.Name, cfg.RoutingKey)

		if err = c.channel.QueueBind(
			queue.Name,           // name of the queue
			cfg.RoutingKey,       // bindingKey
			cfg.Exchange.Name,    // sourceExchange
			false,                // noWait
			cfg.RoutingArguments, // arguments
		); err != nil {
			return fmt.Errorf("queue Bind: %s", err)
		}

		c.log.Infof("Queue bound to Exchange, starting Consume (consumer tag %q)", c.tag)
	} else {
		c.log.Infof("Starting Consume (consumer tag %q)", c.tag)
	}

	err = c.channel.Qos(cfg.PrefetchCount, 0, true)

	if err != nil {
		return fmt.Errorf("qos: %s", err)
	}

	deliveries, err := c.channel.Consume(
		queue.Name, // name
		c.tag,      // consumerTag,
		!cfg.Ack,   // noAck
		false,      // exclusive
		false,      // noLocal
		false,      // noWait
		nil,        // arguments
	)
	if err != nil {
		return fmt.Errorf("queue Consume: %s", err)
	}

	go handle(c, deliveries, c.done)

	return nil
}

// NewConsumer creates a consumer from configuration
func NewConsumer(cfg *common.Config, client beat.Client, name string, version string) *Consumer {

	var consumerConfig config.ConsumerConfig

	mode, _ := cfg.String("mode", -1)

	var maker func(c *Consumer, d amqp.Delivery) *beat.Event

	switch mode {
	case trace:
		maker = func(c *Consumer, d amqp.Delivery) *beat.Event {
			return makeTraceEvent(c, d, consumerConfig.DocumentType, func(c *Consumer, documentType string, rmqEvent RabbitMQEvent) common.MapStr {
				return common.MapStr{
					"type":     documentType,
					"rabbitmq": rmqEvent,
				}
			})
		}
		consumerConfig = config.DefaultTracerConsumerConfig
	case evo_trace:
		maker = func(c *Consumer, d amqp.Delivery) *beat.Event {
			return makeTraceEvent(c, d, consumerConfig.DocumentType, transformEvoTrace)
		}
		consumerConfig = config.DefaultTracerConsumerConfig
	case log:
		maker = func(c *Consumer, d amqp.Delivery) *beat.Event {
			return makeLogEvent(c, d, consumerConfig.DocumentType)
		}
		consumerConfig = config.DefaultLoggerConsumerConfig
	case event:
		maker = func(c *Consumer, d amqp.Delivery) *beat.Event {
			return makeEventEvent(c, d, consumerConfig.DocumentType)
		}
		consumerConfig = config.DefaultEventConsumerConfig
	default:
		maker = func(c *Consumer, d amqp.Delivery) *beat.Event {
			return makeEvent(c, d, consumerConfig.DocumentType, consumerConfig.Queue.Name)
		}
		consumerConfig = config.DefaultConsumerConfig
	}

	if err := cfg.Unpack(&consumerConfig); err != nil {
		return nil
	}
	log := logp.NewLogger(fmt.Sprintf("consumer.%s", consumerConfig.Connection.Name))
	log.Infof("Configuring consumer", consumerConfig.Connection.Name)
	log.Infof("Exchange: %s", consumerConfig.Exchange.Name)
	log.Infof("Routing Key: %s", consumerConfig.RoutingKey)
	log.Infof("Queue: %s", consumerConfig.Queue.Name)

	c := &Consumer{
		conn:     nil,
		channel:  nil,
		tag:      fmt.Sprintf("%s--%s--%s", consumerConfig.Exchange.Name, consumerConfig.RoutingKey, consumerConfig.Queue.Name),
		done:     make(chan error),
		client:   client,
		maker:    maker,
		config:   consumerConfig,
		name:     name,
		version:  version,
		log:      log,
		shutdown: false,
	}

	return c
}

// Shutdown cancels consumer and closes connection
func (c *Consumer) Shutdown() error {
	c.shutdown = true
	// If we are not waiting to reconnect
	if c.channel != nil {
		// will close() the deliveries channel
		if err := c.channel.Cancel(c.tag, true); err != nil {
			return fmt.Errorf("consumer cancel failed: %s", err)
		}

		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("AMQP connection close error: %s", err)
		}

		defer c.log.Infof("AMQP shutdown OK")

		c.client.Close()

		// wait for handle() to exit
		return <-c.done
	}

	c.log.Infof("Stopping in reconnect")
	// Cancel wait to reconnect
	c.done <- nil
	return nil
}

func processDelivery(consumer *Consumer, d amqp.Delivery) {
	event := consumer.maker(consumer, d)
	if event != nil {
		common.AddTags(event.Meta, consumer.config.EventMetadata.Tags)
		common.MergeFields(event.Fields, consumer.config.EventMetadata.Fields, consumer.config.EventMetadata.FieldsUnderRoot)

		consumer.client.Publish(*event)
	}
}

func handle(consumer *Consumer, deliveries <-chan amqp.Delivery, done chan error) {
	ackCounter := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	var lastDelivery *amqp.Delivery
DeliveryLoop:
	for {
		select {
		case d, ok := <-deliveries:
			if ok {
				consumer.log.Debugf("got %dB delivery: [%v] %q", len(d.Body), d.DeliveryTag, d.Body)
				processDelivery(consumer, d)
				lastDelivery = &d
				if consumer.config.Ack {
					if (consumer.config.PrefetchCount == 0 && ackCounter == 512) || (consumer.config.PrefetchCount != 0 && ackCounter == consumer.config.PrefetchCount-1) {
						consumer.log.Debugf("ACK after %d", ackCounter)
						ackCounter = 0
						lastDelivery = nil

						d.Ack(true)
					} else {
						ackCounter += 1

						ticker.Stop()
						ticker = time.NewTicker(100 * time.Millisecond)
					}
				}
			} else {
				break DeliveryLoop
			}
		case <-ticker.C:
			if lastDelivery != nil {
				consumer.log.Debugf("ACK on timeout. Counter was %d", ackCounter)
				ackCounter = 0
				lastDelivery.Ack(true)
				lastDelivery = nil
				ticker.Stop()
			}

		}
	}
	consumer.log.Warn("handle: deliveries channel closed")
	if !consumer.shutdown {
		consumer.log.Infof("External channel closure -> retry consuming")
		deliveries, err := consumer.channel.Consume(
			consumer.config.Queue.Name, // name
			consumer.tag,               // consumerTag,
			!consumer.config.Ack,       // noAck
			false,                      // exclusive
			false,                      // noLocal
			false,                      // noWait
			nil,                        // arguments
		)
		if err != nil {
			consumer.reconnect()
		}

		go handle(consumer, deliveries, consumer.done)
	} else {
		consumer.log.Info("handle: Done")
		done <- nil
	}
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

func makeEvent(c *Consumer, d amqp.Delivery, documentType string, queue string) *beat.Event {
	payloadBody, _ := decode(d.Body, d.ContentEncoding)

	return &beat.Event{
		Timestamp: time.Now(),
		Fields: common.MapStr{
			"type": documentType,
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
		},
	}
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
	var num int
	switch v := something.(type) {
	default:
		logp.Err("unexpected type %T when converting to bool", v)
		return nil
	case int8:
		num = int(something.(int8))
	case uint8:
		num = int(something.(uint8))
	case int16:
		num = int(something.(int16))
	case uint16:
		num = int(something.(uint16))
	case int32:
		num = int(something.(int32))
	case uint32:
		num = int(something.(uint32))
	case int64:
		num = int(something.(int64))
	case uint64:
		num = int(something.(uint64))
	case int:
		num = something.(int)
	}
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

func extractBool(something interface{}) bool {
	if something == nil {
		return false
	}
	switch v := something.(type) {
	default:
		num := optionalInt(something)
		if num == nil {
			logp.Err("unexpected type %T when converting to bool", v)
		}
		return *num != 0
	case bool:
		return something.(bool)
	}
}

func optionalTable(something interface{}) amqp.Table {
	if something == nil {
		return amqp.Table{}
	}
	return something.(amqp.Table)
}

func makeTraceEvent(c *Consumer, d amqp.Delivery, documentType string, transform func(c *Consumer, documentType string, event RabbitMQEvent) common.MapStr) *beat.Event {

	switch d.Headers["exchange_name"] {
	case
		"amq.rabbitmq.event",
		"amq.rabbitmq.trace",
		"amq.rabbitmq.log":
		return nil
	}

	realProperties := optionalTable(d.Headers["properties"])
	realHeaders := optionalTable(realProperties["headers"])

	c.log.Debugf("RK: %s", d.RoutingKey)

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
	timestamp := time.Now()
	if hasRealTimestamp {
		ts := time.Unix(int64(mayBeRealTimestamp.(int32)), 0)
		realTimestamp = &ts
	}
	var hash = sha256.Sum256(d.Body)
	return &beat.Event{
		Timestamp: timestamp,
		Fields: transform(c, documentType,
			RabbitMQEvent{
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
				Redelivered:  extractBool(d.Headers["redelivered"]),
				RoutingKey:   (*optionalStringArray(d.Headers["routing_keys"]))[0],
				VHost:        c.config.Connection.Vhost,
				Payload: Payload{
					Size: len(d.Body),
					Body: payloadBody,
					Hash: hex.EncodeToString(hash[:]),
				},
			}),
	}
}

func transformEvoTrace(c *Consumer, documentType string, rmqEvent RabbitMQEvent) common.MapStr {
	var fields = common.MapStr{}
	common.AddTags(fields, []string{"rmq_message"})
	var deviceId = optionalString(rmqEvent.Headers["device_id"])
	if deviceId != nil {
		var deviceIdParts = strings.SplitN(*deviceId, "::", 2)
		var userType = strings.ToUpper(deviceIdParts[0])
		var userId, userIdErr = strconv.Atoi(deviceIdParts[1])
		if userIdErr == nil {
			fields.Put("user_type", userType)
			fields.Put("user_id", userId)
		}
	}

	var familyId = optionalString(rmqEvent.Headers["family_id"])
	if familyId != nil {
		var familyIdInt, familyIdErr = strconv.Atoi(*familyId)
		if familyIdErr == nil {
			fields.Put("family_id", familyIdInt)
		}
	}

	var requestId = rmqEvent.Properties.MessageID
	if requestId != nil {
		fields.Put("request_id", *requestId)
	}

	if rmqEvent.Properties.ContentType != nil && *rmqEvent.Properties.ContentType == "application/json" {
		var body map[string]interface{}
		err := json.Unmarshal([]byte(rmqEvent.Payload.Body), &body)

		if err == nil {
			var key = ""
			if len(body) == 1 {
				for k := range body {
					key = k
					break
				}
			}

			if key != "" {
				content := body[key].(map[string]interface{})
				timestamp, ok := content["timestamp"]
				if ok {
					ts, err := time.Parse(time.RFC3339, timestamp.(string))
					if err == nil {
						rmqEvent.Payload.Timestamp = &ts
					}
				}
				if key == "create" || key == "update" || key == "delete" {
					key = key + "_" + content["type"].(string)
				} else if key == "parameter" {
					key = key + "_" + content["name"].(string)
					rawValue := content["value"].(string)
					var value = common.MapStr{"raw": rawValue}
					intValue, intErr := strconv.Atoi(rawValue)
					if intErr == nil {
						value.Put("long", intValue)
					} else {
						floatValue, floatErr := strconv.ParseFloat(rawValue, 64)
						if floatErr == nil {
							value.Put("float", floatValue)
						}
					}

					content["value"] = value
				}

				body = map[string]interface{}{key: content,}
				rmqEvent.Payload.Type = &key
			}
			rmqEvent.Payload.Json = body
		}
		if c.config.IncludeMessage {
			fields.Put("message", fmt.Sprintf("JSON Message: %s", rmqEvent.Payload.Body))
		}
	} else if rmqEvent.Properties.ContentType != nil && strings.HasPrefix(*rmqEvent.Properties.ContentType, "text/") {
		var key = "text"
		rmqEvent.Payload.Type = &key
		if c.config.IncludeMessage {
			fields.Put("message", "Message: "+rmqEvent.Payload.Body)
		}
	} else {
		var key = "binary"
		rmqEvent.Payload.Type = &key
		rmqEvent.Payload.Body = "<<binary>>"
		if c.config.IncludeMessage {
			fields.Put("message", fmt.Sprintf("Binary Message (%d bytes): %s", rmqEvent.Payload.Size, rmqEvent.Payload.Hash))
		}
	}
	fields["rabbitmq"] = rmqEvent

	return fields
}

func makeLogEvent(c *Consumer, d amqp.Delivery, documentType string) *beat.Event {
	payloadBody, _ := decode(d.Body, d.ContentEncoding)

	var timestamp time.Time

	timestampString := payloadBody[:23]
	bodyTimestamp, err := time.ParseInLocation("2006-01-02 15:04:05.999", timestampString, time.Local)
	if err == nil {
		timestamp = bodyTimestamp
	} else if !d.Timestamp.IsZero() {
		timestamp = d.Timestamp
	} else {
		timestamp = time.Now()
	}

	payloadBody = payloadBody[24:]

	severityStart := strings.Index(payloadBody, "[")
	if severityStart >= 0 {
		severityEnd := strings.Index(payloadBody[severityStart:], "]")
		if severityEnd > 0 {
			// severity := payloadBody[severityStart+1 : severityStart+severityEnd]

			payloadBody = payloadBody[severityStart+severityEnd+1:]
		}
	}

	var pid string
	pidStart := strings.Index(payloadBody, "<")
	if pidStart >= 0 {
		pidEnd := strings.Index(payloadBody[pidStart:], ">")
		if pidEnd > 0 {
			pid = payloadBody[pidStart+1 : pidStart+pidEnd]

			payloadBody = payloadBody[pidStart+pidEnd+1:]
		}
	}

	payloadBody = strings.TrimSpace(payloadBody)

	// [date," ",time," ",color,"[",severity,"] ", {pid,[]}, " ",message,"\n"]

	return &beat.Event{
		Timestamp: timestamp,
		Fields: common.MapStr{
			"type":    documentType,
			"message": payloadBody,
			"pid":     pid,
			"level":   d.RoutingKey,
			"node":    optionalString(d.Headers["node"]),
		},
	}
}

func makeEventEvent(c *Consumer, d amqp.Delivery, documentType string) *beat.Event {
	for _, ex := range c.config.Exclude {
		if ex == d.RoutingKey {
			return nil
		}
	}
	var eventDetails = common.MapStr{
		"type": d.RoutingKey,
	}

	clientProperties, ok := d.Headers["client_properties"]
	if ok {
		properties := clientProperties.([]interface{})
		var props = make(map[string]interface{})
		for _, prop := range properties {
			key, value, _ := parseAmqpStructure(prop.(string))
			if key != "" {
				props[key] = value
			}
		}
		d.Headers["client_properties"] = props
	}

	fixIp(&d.Headers, "host")
	fixIp(&d.Headers, "peer_host")

	fixPort(&d.Headers, "port")
	fixPort(&d.Headers, "peer_port")

	protocol, ok := d.Headers["protocol"]
	if ok {
		d.Headers["protocol"] = parseProtocol(protocol.(string))
	}

	/*error, ok := d.Headers["error"]
	if ok {
		d.Headers["error"] = parseProtocol(protocol.(string))
	}*/

	value, ok := d.Headers["value"]
	if ok {
		switch value.(type) {
		default:
			b, err := json.Marshal(value)
			if err != nil {
				d.Headers["value"] = fmt.Sprintf("%#v", value)
			} else {
				d.Headers["value"] = string(b)
			}
		case string:
			// Do nothing
		}
	}

	connectedAt := optionalInt(d.Headers["connected_at"])
	if connectedAt != nil {
		d.Headers["connected_at"] = parseTimestampMs(*connectedAt)
	}

	var timestamp time.Time
	timestampInMs := optionalInt(d.Headers["timestamp_in_ms"])
	if timestampInMs != nil {
		timestamp = parseTimestampMs(*timestampInMs)
		delete(d.Headers, "timestamp_in_ms")
	} else if !d.Timestamp.IsZero() {
		timestamp = d.Timestamp
	} else {
		timestamp = time.Now()
	}

	eventDetails.Put(d.RoutingKey, d.Headers)

	fields := common.MapStr{
		"type": documentType,
		"rabbitmq": common.MapStr{
			"event": eventDetails,
		},
		"node": optionalString(d.Headers["node"]),
	}

	user, ok := d.Headers["user"]
	if !ok {
		user, ok = d.Headers["user_who_performed_action"]
	}
	if ok && strings.Contains(user.(string), "::") {
		userName := user.(string)
		var deviceIdParts = strings.SplitN(userName, "::", 2)
		var userType = strings.ToUpper(deviceIdParts[0])
		var userId, userIdErr = strconv.Atoi(deviceIdParts[1])
		if userIdErr == nil {
			fields.Put("user_type", userType)
			fields.Put("user_id", userId)
		}
	}

	if c.config.IncludeMessage {
		fields.Put("message", "Event: "+d.RoutingKey)
	}

	return &beat.Event{
		Timestamp: timestamp,
		Fields:    fields,
	}
}

func parseTimestampMs(timestampInMs int) time.Time {
	sec := timestampInMs / 1000
	nsec := ((timestampInMs) % 1000) * 1000000
	return time.Unix(int64(sec), int64(nsec))
}

func fixIp(h *amqp.Table, key string) {
	host, ok := (*h)[key]
	if ok {
		hostString := host.(string)
		if hostString == "unknown" {
			delete(*h, key)
		} else {
			(*h)[key] = parseIp(hostString)
		}
	}
}

func fixPort(h *amqp.Table, key string) {
	port, ok := (*h)[key]
	if ok {
		switch port.(type) {
		default:
			intPort := optionalInt(port)
			if intPort == nil {
				delete(*h, key)
			} else {
				(*h)[key] = intPort
			}
		case string:
			portString := port.(string)
			if portString == "unknown" {
				delete(*h, key)
			} else {
				p, err := strconv.ParseInt(portString, 10, 16)
				if err != nil {
					(*h)[key] = p
				} else {
					logp.Err("Can not parse port number %s from %s: %s", portString, key, err)
					delete(*h, key)
				}
			}
		}
	}
}

func parseIp(str string) string {
	s := strings.TrimSuffix(strings.TrimPrefix(str, "{"), "}")
	hostParts := strings.Split(s, ",")
	if len(hostParts) == 4 {
		return strings.Join(hostParts, ".")
	} else {
		return strings.Join(hostParts, ":")
	}
}

func parseProtocol(str string) string {
	s := strings.TrimSuffix(strings.TrimPrefix(str, "{"), "}")
	protocolParts := strings.Split(s, ",")
	return strings.Join(protocolParts, ".")
}

func parseAmqpStructure(str string) (string, interface{}, string) {
	startPos := strings.Index(str, "{<<\"")
	if startPos < 0 {
		return "", nil, str
	}
	str = str[startPos+4:]
	nameEndPos := strings.Index(str, "\">>,")
	if nameEndPos < 0 {
		return "", nil, str
	}
	key := str[0:nameEndPos]
	str = strings.TrimSpace(str[nameEndPos+4:])
	typeEndPos := strings.Index(str, ",")
	if typeEndPos < 0 {
		return "", nil, str
	}
	valueType := str[:typeEndPos]
	str = strings.TrimSpace(str[typeEndPos+1:])
	var parser func(s string) (interface{}, string)
	switch valueType {
	case "longstr", "shortstr":
		parser = parseAmqpString
	case "bool":
		parser = parseAmqpBool
	case "table":
		parser = parseAmqpTable
	case "short", "longlong", "long", "octet":
		parser = parseAmqpNumber
	default:
		parser = func(s string) (interface{}, string) {
			return nil, s
		}
	}

	value, left := parser(str)
	return key, value, strings.TrimPrefix(left, "}")
}

func parseAmqpNumber(value string) (interface{}, string) {
	endPos := strings.Index(value, "}")
	if endPos < 0 {
		return "", value
	}
	v, err := strconv.Atoi(value[:endPos])
	if err != nil {
		return nil, value
	}
	return v, value[endPos:]
}

func parseAmqpString(value string) (interface{}, string) {
	startPos := strings.Index(value, "<<\"")
	if startPos < 0 {
		return "", value
	}
	value = value[startPos+3:]
	endPos := strings.Index(value, "\">>")
	if endPos < 0 {
		return "", value
	}

	return value[:endPos], value[endPos+3:]
}

func parseAmqpBool(value string) (interface{}, string) {
	if strings.HasPrefix(value, "true") {
		return true, value[4:]
	} else if strings.HasPrefix(value, "false") {
		return false, value[5:]
	} else {
		return nil, value
	}
}

func parseAmqpTable(str string) (interface{}, string) {
	startPos := strings.Index(str, "[")
	if startPos < 0 {
		return "", str
	}
	str = strings.TrimSpace(str[startPos+1:])
	var values = make(map[string]interface{})

	for len(str) > 1 && str[0] != ']' {
		var key string
		var value interface{}
		key, value, str = parseAmqpStructure(str)
		values[key] = value
		str = strings.TrimPrefix(str, ",")
	}

	return values, str
}
