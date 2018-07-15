package beater

import "time"

// AMQPProperties represents AMQP message properties
type AMQPProperties struct {
	ContentType     *string    `json:"content_type,omitempty"`     // MIME content type
	ContentEncoding *string    `json:"content_encoding,omitempty"` // MIME content encoding
	DeliveryMode    *int       `json:"delivery_mode,omitempty"`    // queue implementation use - non-persistent (1) or persistent (2)
	Priority        *int       `json:"priority,omitempty"`         // queue implementation use - 0 to 9
	CorrelationID   *string    `json:"correlation_id,omitempty"`   // application use - correlation identifier
	ReplyTo         *string    `json:"reply_to,omitempty"`         // application use - address to to reply to (ex: RPC)
	Expiration      *string    `json:"expiration,omitempty"`       // implementation use - message expiration spec
	MessageID       *string    `json:"message_id,omitempty"`       // application use - message identifier
	Timestamp       *time.Time `json:"timestamp,omitempty"`        // application use - message timestamp
	Type            *string    `json:"type,omitempty"`             // application use - message type name
	UserID          *string    `json:"user_id,omitempty"`          // application use - creating user - should be authenticated user
	AppID           *string    `json:"app_id,omitempty"`           // application use - creating application id
}

// Payload is a container for message payload
type Payload struct {
	Size      int         `json:"size,omitempty"`
	Body      string      `json:"body,omitempty"`
	Json      interface{} `json:"json,omitempty"`
	Type      *string     `json:"type,omitempty"`
	Timestamp *time.Time  `json:"timestamp,omitempty"`
}

// RabbitMQEvent contains information about a single AMQP message (or event relating to the message in tracer mode)
type RabbitMQEvent struct {
	Properties   AMQPProperties         `json:"properties"`
	Headers      map[string]interface{} `json:"headers"`
	Action       string                 `json:"action"`
	Exchange     string                 `json:"exchange"`
	RoutingKey   string                 `json:"routing_key"`
	Queue        *string                `json:"queue"`
	ConsumerTag  *string                `json:"consumer_tag"`
	Connection   *string                `json:"connection,omitempty"`
	Channel      *int                   `json:"channel,omitempty"`
	User         *string                `json:"user,omitempty"`
	RoutedQueues *[]string              `json:"routed_queues,omitempty"`
	Redelivered  bool                   `json:"redelivered"`
	Payload      Payload                `json:"payload"`
}
