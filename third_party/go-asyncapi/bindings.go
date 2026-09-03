package asyncapi

import "encoding/json"

// ServerBindings contains protocol-specific definitions for a server.
type ServerBindings struct {
	HTTP  *HTTPServerBinding  `json:"http,omitempty" yaml:"http,omitempty"`
	WS    *WSServerBinding    `json:"ws,omitempty" yaml:"ws,omitempty"`
	Kafka *KafkaServerBinding `json:"kafka,omitempty" yaml:"kafka,omitempty"`
	AMQP  *AMQPServerBinding  `json:"amqp,omitempty" yaml:"amqp,omitempty"`
	MQTT  *MQTTServerBinding  `json:"mqtt,omitempty" yaml:"mqtt,omitempty"`
	NATS  *NATSServerBinding  `json:"nats,omitempty" yaml:"nats,omitempty"`
	// Raw captures any additional/unknown bindings
	Raw map[string]json.RawMessage `json:"-" yaml:"-"`
}

// ChannelBindings contains protocol-specific definitions for a channel.
type ChannelBindings struct {
	HTTP  *HTTPChannelBinding        `json:"http,omitempty" yaml:"http,omitempty"`
	WS    *WSChannelBinding          `json:"ws,omitempty" yaml:"ws,omitempty"`
	Kafka *KafkaChannelBinding       `json:"kafka,omitempty" yaml:"kafka,omitempty"`
	AMQP  *AMQPChannelBinding        `json:"amqp,omitempty" yaml:"amqp,omitempty"`
	MQTT  *MQTTChannelBinding        `json:"mqtt,omitempty" yaml:"mqtt,omitempty"`
	NATS  *NATSChannelBinding        `json:"nats,omitempty" yaml:"nats,omitempty"`
	Raw   map[string]json.RawMessage `json:"-" yaml:"-"`
}

// OperationBindings contains protocol-specific definitions for an operation.
type OperationBindings struct {
	HTTP  *HTTPOperationBinding      `json:"http,omitempty" yaml:"http,omitempty"`
	WS    *WSOperationBinding        `json:"ws,omitempty" yaml:"ws,omitempty"`
	Kafka *KafkaOperationBinding     `json:"kafka,omitempty" yaml:"kafka,omitempty"`
	AMQP  *AMQPOperationBinding      `json:"amqp,omitempty" yaml:"amqp,omitempty"`
	MQTT  *MQTTOperationBinding      `json:"mqtt,omitempty" yaml:"mqtt,omitempty"`
	NATS  *NATSOperationBinding      `json:"nats,omitempty" yaml:"nats,omitempty"`
	Raw   map[string]json.RawMessage `json:"-" yaml:"-"`
}

// MessageBindings contains protocol-specific definitions for a message.
type MessageBindings struct {
	HTTP  *HTTPMessageBinding        `json:"http,omitempty" yaml:"http,omitempty"`
	WS    *WSMessageBinding          `json:"ws,omitempty" yaml:"ws,omitempty"`
	Kafka *KafkaMessageBinding       `json:"kafka,omitempty" yaml:"kafka,omitempty"`
	AMQP  *AMQPMessageBinding        `json:"amqp,omitempty" yaml:"amqp,omitempty"`
	MQTT  *MQTTMessageBinding        `json:"mqtt,omitempty" yaml:"mqtt,omitempty"`
	NATS  *NATSMessageBinding        `json:"nats,omitempty" yaml:"nats,omitempty"`
	Raw   map[string]json.RawMessage `json:"-" yaml:"-"`
}

// HTTP Bindings

type HTTPServerBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type HTTPChannelBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type HTTPOperationBinding struct {
	Method         string     `json:"method,omitempty" yaml:"method,omitempty"`
	Query          *SchemaRef `json:"query,omitempty" yaml:"query,omitempty"`
	BindingVersion string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type HTTPMessageBinding struct {
	Headers        *SchemaRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	StatusCode     *int       `json:"statusCode,omitempty" yaml:"statusCode,omitempty"`
	BindingVersion string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

// WebSocket Bindings

type WSServerBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type WSChannelBinding struct {
	Method         string     `json:"method,omitempty" yaml:"method,omitempty"`
	Query          *SchemaRef `json:"query,omitempty" yaml:"query,omitempty"`
	Headers        *SchemaRef `json:"headers,omitempty" yaml:"headers,omitempty"`
	BindingVersion string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type WSOperationBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type WSMessageBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

// Kafka Bindings

type KafkaServerBinding struct {
	SchemaRegistryURL    string `json:"schemaRegistryUrl,omitempty" yaml:"schemaRegistryUrl,omitempty"`
	SchemaRegistryVendor string `json:"schemaRegistryVendor,omitempty" yaml:"schemaRegistryVendor,omitempty"`
	BindingVersion       string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type KafkaChannelBinding struct {
	Topic              string `json:"topic,omitempty" yaml:"topic,omitempty"`
	Partitions         *int   `json:"partitions,omitempty" yaml:"partitions,omitempty"`
	Replicas           *int   `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	TopicConfiguration any    `json:"topicConfiguration,omitempty" yaml:"topicConfiguration,omitempty"`
	BindingVersion     string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type KafkaOperationBinding struct {
	GroupID        *SchemaRef `json:"groupId,omitempty" yaml:"groupId,omitempty"`
	ClientID       *SchemaRef `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	BindingVersion string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type KafkaMessageBinding struct {
	Key                     *SchemaRef `json:"key,omitempty" yaml:"key,omitempty"`
	SchemaIDLocation        string     `json:"schemaIdLocation,omitempty" yaml:"schemaIdLocation,omitempty"`
	SchemaIDPayloadEncoding string     `json:"schemaIdPayloadEncoding,omitempty" yaml:"schemaIdPayloadEncoding,omitempty"`
	SchemaLookupStrategy    string     `json:"schemaLookupStrategy,omitempty" yaml:"schemaLookupStrategy,omitempty"`
	BindingVersion          string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

// AMQP Bindings

type AMQPServerBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type AMQPChannelBinding struct {
	Is             string `json:"is,omitempty" yaml:"is,omitempty"` // queue or routingKey
	Exchange       any    `json:"exchange,omitempty" yaml:"exchange,omitempty"`
	Queue          any    `json:"queue,omitempty" yaml:"queue,omitempty"`
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type AMQPOperationBinding struct {
	Expiration     int      `json:"expiration,omitempty" yaml:"expiration,omitempty"`
	UserID         string   `json:"userId,omitempty" yaml:"userId,omitempty"`
	CC             []string `json:"cc,omitempty" yaml:"cc,omitempty"`
	Priority       *int     `json:"priority,omitempty" yaml:"priority,omitempty"`
	DeliveryMode   *int     `json:"deliveryMode,omitempty" yaml:"deliveryMode,omitempty"`
	Mandatory      bool     `json:"mandatory,omitempty" yaml:"mandatory,omitempty"`
	BCC            []string `json:"bcc,omitempty" yaml:"bcc,omitempty"`
	Timestamp      bool     `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	Ack            bool     `json:"ack,omitempty" yaml:"ack,omitempty"`
	BindingVersion string   `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type AMQPMessageBinding struct {
	ContentEncoding string `json:"contentEncoding,omitempty" yaml:"contentEncoding,omitempty"`
	MessageType     string `json:"messageType,omitempty" yaml:"messageType,omitempty"`
	BindingVersion  string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

// MQTT Bindings

type MQTTServerBinding struct {
	ClientID              string `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	CleanSession          bool   `json:"cleanSession,omitempty" yaml:"cleanSession,omitempty"`
	LastWill              any    `json:"lastWill,omitempty" yaml:"lastWill,omitempty"`
	KeepAlive             *int   `json:"keepAlive,omitempty" yaml:"keepAlive,omitempty"`
	SessionExpiryInterval *int   `json:"sessionExpiryInterval,omitempty" yaml:"sessionExpiryInterval,omitempty"`
	MaximumPacketSize     *int   `json:"maximumPacketSize,omitempty" yaml:"maximumPacketSize,omitempty"`
	BindingVersion        string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type MQTTChannelBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type MQTTOperationBinding struct {
	QoS                   *int   `json:"qos,omitempty" yaml:"qos,omitempty"`
	Retain                bool   `json:"retain,omitempty" yaml:"retain,omitempty"`
	MessageExpiryInterval *int   `json:"messageExpiryInterval,omitempty" yaml:"messageExpiryInterval,omitempty"`
	BindingVersion        string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type MQTTMessageBinding struct {
	PayloadFormatIndicator *int       `json:"payloadFormatIndicator,omitempty" yaml:"payloadFormatIndicator,omitempty"`
	CorrelationData        *SchemaRef `json:"correlationData,omitempty" yaml:"correlationData,omitempty"`
	ContentType            string     `json:"contentType,omitempty" yaml:"contentType,omitempty"`
	ResponseTopic          string     `json:"responseTopic,omitempty" yaml:"responseTopic,omitempty"`
	BindingVersion         string     `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

// NATS Bindings

type NATSServerBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type NATSChannelBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type NATSOperationBinding struct {
	Queue          string `json:"queue,omitempty" yaml:"queue,omitempty"`
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}

type NATSMessageBinding struct {
	BindingVersion string `json:"bindingVersion,omitempty" yaml:"bindingVersion,omitempty"`
}
