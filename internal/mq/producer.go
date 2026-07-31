package mq

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"worksentry/internal/config"
)

var ErrDisabled = errors.New("mq producer disabled")

type Message struct {
	Key   string
	Value []byte
}

type Producer interface {
	Publish(ctx context.Context, message Message) error
	Close() error
}

type DisabledProducer struct{}

func (DisabledProducer) Publish(context.Context, Message) error {
	return ErrDisabled
}

func (DisabledProducer) Close() error {
	return nil
}

type KafkaProducer struct {
	writer *kafka.Writer
}

func NewProducer(cfg config.MQConfig) Producer {
	if !cfg.Enabled {
		return DisabledProducer{}
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Provider), "kafka") {
		return DisabledProducer{}
	}
	if len(cfg.Brokers) == 0 || strings.TrimSpace(cfg.Topic) == "" {
		return DisabledProducer{}
	}
	timeout := time.Duration(cfg.WriteTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &KafkaProducer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        cfg.Topic,
			Balancer:     &kafka.Hash{},
			BatchTimeout: 10 * time.Millisecond,
			WriteTimeout: timeout,
		},
	}
}

func (p *KafkaProducer) Publish(ctx context.Context, message Message) error {
	if p == nil || p.writer == nil {
		return ErrDisabled
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(message.Key),
		Value: message.Value,
		Time:  time.Now(),
	})
}

func (p *KafkaProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
