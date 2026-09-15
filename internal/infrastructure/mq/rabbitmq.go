package mq

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	"github.com/inumoroha/video-feed/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 封装互动事件的发布与消费通道。
type RabbitMQ struct {
	conn    *amqp.Connection
	publish *amqp.Channel
	consume *amqp.Channel
	cfg     config.RabbitMQConfig
}

// NewRabbitMQ 连接 RabbitMQ 并声明互动事件交换机、队列及绑定。
func NewRabbitMQ(cfg config.RabbitMQConfig) (*RabbitMQ, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("rabbitmq url is empty")
	}
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	p, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	c, err := conn.Channel()
	if err != nil {
		_ = p.Close()
		_ = conn.Close()
		return nil, err
	}
	r := &RabbitMQ{conn: conn, publish: p, consume: c, cfg: cfg}
	if err := r.ensureTopology(); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

// Close 关闭 RabbitMQ 通道和连接。
func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publish != nil {
		_ = r.publish.Close()
	}
	if r.consume != nil {
		_ = r.consume.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// PublishActionChanged 发布一条持久化的互动变更事件。
func (r *RabbitMQ) PublishActionChanged(ctx context.Context, event *applicationinteraction.ActionChangedEvent) error {
	if event == nil {
		return nil
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publish.PublishWithContext(ctx, r.cfg.InteractionExchange, r.cfg.ActionChangedRouting, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: event.EventID, Body: body})
}

// ConsumeActionChanged 消费互动变更事件并按处理结果确认或重试消息。
func (r *RabbitMQ) ConsumeActionChanged(ctx context.Context, handler func(context.Context, *applicationinteraction.ActionChangedEvent) error) error {
	ds, err := r.consume.ConsumeWithContext(ctx, r.cfg.ActionChangedQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for d := range ds {
			var e applicationinteraction.ActionChangedEvent
			if json.Unmarshal(d.Body, &e) != nil {
				_ = d.Nack(false, false)
				continue
			}
			if handler(ctx, &e) != nil {
				_ = d.Nack(false, true)
			} else {
				_ = d.Ack(false)
			}
		}
	}()
	return nil
}

func (r *RabbitMQ) ensureTopology() error {
	if err := r.publish.ExchangeDeclare(r.cfg.InteractionExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := r.publish.QueueDeclare(r.cfg.ActionChangedQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return r.publish.QueueBind(r.cfg.ActionChangedQueue, r.cfg.ActionChangedRouting, r.cfg.InteractionExchange, false, nil)
}
func normalizeConfig(c config.RabbitMQConfig) config.RabbitMQConfig {
	if strings.TrimSpace(c.InteractionExchange) == "" {
		c.InteractionExchange = "gcfeed.interaction"
	}
	if strings.TrimSpace(c.ActionChangedQueue) == "" {
		c.ActionChangedQueue = "gcfeed.interaction.action_changed"
	}
	if strings.TrimSpace(c.ActionChangedRouting) == "" {
		c.ActionChangedRouting = "interaction.action_changed"
	}
	return c
}
