package beater

import (
	"github.com/elastic/beats/libbeat/common"
	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// TestCompleteEvent verifies the returned event against Delivery for normal mode
func TestCompleteEvent(t *testing.T) {

	event := makeEvent(amqp.Delivery{
		Headers: amqp.Table{
			"number": 3,
			"str":    "String",
		},

		ContentType:     "application/json",
		ContentEncoding: "utf8",
		DeliveryMode:    1,
		Priority:        5,
		CorrelationId:   "correlation-id",
		ReplyTo:         "reply-to",
		Expiration:      "expiration",
		MessageId:       "message-id",
		Timestamp:       time.Date(2017, 3, 8, 20, 27, 3, 0, time.Local),
		Type:            "type",
		UserId:          "user-id",
		AppId:           "app-id",
		ConsumerTag:     "consumer-tag",
		Exchange:        "exchange",
		RoutingKey:      "routing-key",
		Redelivered:     true,

		Body: []byte("body"),
	}, "doctype", "queuename")

	assert.NotNil(t, event)

	trueEvent := *event

	t.Logf("event: %+v", trueEvent.StringToPrint())
	assert.EqualValues(t, time.Date(2017, 3, 8, 20, 27, 3, 0, time.Local), trueEvent["@timestamp"])
	assert.EqualValues(t, "doctype", trueEvent["type"])

	rabbitmq := trueEvent["rabbitmq"].(RabbitMQEvent)
	assert.EqualValues(t, "receive", rabbitmq.Action)
	assert.EqualValues(t, "exchange", rabbitmq.Exchange)
	assert.EqualValues(t, "routing-key", rabbitmq.RoutingKey)

	assert.EqualValues(t, "queuename", *rabbitmq.Queue)
	assert.EqualValues(t, "consumer-tag", *rabbitmq.ConsumerTag)
	assert.Nil(t, rabbitmq.Connection)
	assert.Nil(t, rabbitmq.Channel)
	assert.Nil(t, rabbitmq.User)
	assert.Nil(t, rabbitmq.RoutedQueues)
	assert.True(t, rabbitmq.Redelivered)

	payload := rabbitmq.Payload
	assert.EqualValues(t, 4, payload.Size)
	assert.EqualValues(t, "body", payload.Body)

	headers := rabbitmq.Headers
	assert.EqualValues(t, 3, headers["number"])
	assert.EqualValues(t, "String", headers["str"])
}

// TestCompleteTraceEvent verifies the returned event against Delivery for tracer mode
func TestCompleteTraceEvent(t *testing.T) {

	event := makeTraceEvent(amqp.Delivery{
		Headers: amqp.Table{
			"exchange_name": "some.exchange",
			"routing_keys":  []interface{}{"rk"},
			"properties": amqp.Table{
				"content_type":     "application/json",
				"content_encoding": "utf8",
				"delivery_mode":    1,
				"priority":         5,
				"correlation_id":   "correlation-id",
				"reply_to":         "reply-to",
				"expiration":       "expiration",
				"message_id":       "message-id",
				"timestamp":        int32(1488959176),
				"type":             "type",
				"user_id":          "user-id",
				"app_id":           "app-id",
				"headers": amqp.Table{
					"node": "rabbit@127.0.0.2",
				},
			},
			"node":          "rabbit@127.0.0.1",
			"redelivered":   1,
			"vhost":         "/",
			"connection":    "127.0.0.1:46690 -> 127.0.0.1:5672",
			"channel":       1,
			"user":          "guest",
			"routed_queues": []interface{}{"process"},
		},

		Exchange:    "amq.rabbitmq.trace",
		RoutingKey:  "publish.some.exchange",
		Redelivered: false,

		Body: []byte("traced body"),
	}, "doctype")

	assert.NotNil(t, event)

	trueEvent := *event

	t.Logf("event: %+v", trueEvent.StringToPrint())
	assert.EqualValues(t, common.Time(time.Date(2017, 3, 8, 7, 46, 16, 0, time.UTC).In(time.Local)), trueEvent["@timestamp"])
	assert.EqualValues(t, "doctype", trueEvent["type"])

	rabbitmq := trueEvent["rabbitmq"].(RabbitMQEvent)
	assert.EqualValues(t, "publish", rabbitmq.Action)
	assert.EqualValues(t, "some.exchange", rabbitmq.Exchange)
	assert.EqualValues(t, "rk", rabbitmq.RoutingKey)

	assert.Nil(t, rabbitmq.Queue)
	assert.Nil(t, rabbitmq.ConsumerTag)
	assert.EqualValues(t, "127.0.0.1:46690 -> 127.0.0.1:5672", *rabbitmq.Connection)
	assert.EqualValues(t, 1, *rabbitmq.Channel)
	assert.EqualValues(t, "guest", *rabbitmq.User)
	assert.EqualValues(t, []string{"process"}, *rabbitmq.RoutedQueues)
	assert.True(t, rabbitmq.Redelivered)

	properties := rabbitmq.Properties
	assert.EqualValues(t, "application/json", *properties.ContentType)
	assert.EqualValues(t, "utf8", *properties.ContentEncoding)
	assert.EqualValues(t, 1, *properties.DeliveryMode)
	assert.EqualValues(t, 5, *properties.Priority)
	assert.EqualValues(t, "correlation-id", *properties.CorrelationID)
	assert.EqualValues(t, "reply-to", *properties.ReplyTo)
	assert.EqualValues(t, "expiration", *properties.Expiration)
	assert.EqualValues(t, "message-id", *properties.MessageID)
	assert.EqualValues(t, time.Date(2017, 3, 8, 7, 46, 16, 0, time.UTC).In(time.Local), *properties.Timestamp)
	assert.EqualValues(t, "type", *properties.Type)
	assert.EqualValues(t, "user-id", *properties.UserID)
	assert.EqualValues(t, "app-id", *properties.AppID)

	payload := rabbitmq.Payload
	assert.EqualValues(t, 11, payload.Size)
	assert.EqualValues(t, "traced body", payload.Body)

	headers := rabbitmq.Headers
	assert.EqualValues(t, "rabbit@127.0.0.2", headers["node"])
}

// TestCompleteEvent verifies tracer skips logs
func TestCompleteTraceLogEvent(t *testing.T) {

	event := makeTraceEvent(amqp.Delivery{
		Headers: amqp.Table{
			"exchange_name": "amq.rabbitmq.log",
			"routing_keys":  []string{"rk"},
			"properties": amqp.Table{
				"content_type":     "application/json",
				"content_encoding": "utf8",
				"delivery_mode":    1,
				"priority":         5,
				"correlation_id":   "correlation-id",
				"reply_to":         "reply-to",
				"expiration":       "expiration",
				"message_id":       "message-id",
				"timestamp":        1488959176,
				"type":             "type",
				"user_id":          "user-id",
				"app_id":           "app-id",
				"headers": amqp.Table{
					"node": "rabbit@127.0.0.2",
				},
				"routed_queues": []string{"process"},
			},
			"node":        "rabbit@127.0.0.1",
			"redelivered": 0,
			"vhost":       "/",
			"connection":  "127.0.0.1:46690 -> 127.0.0.1:5672",
			"channel":     1,
			"user":        "guest",
		},

		Exchange:    "amq.rabbitmq.trace",
		RoutingKey:  "publish.exchange-name",
		Redelivered: true,

		Body: []byte("body"),
	}, "doctype")
	assert.Nil(t, event)
}
