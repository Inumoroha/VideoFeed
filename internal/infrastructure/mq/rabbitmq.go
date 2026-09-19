package mq

import (
	"context"
	"encoding/json"
	"errors"
	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	applicationvideo "github.com/inumoroha/video-feed/internal/application/video"
	"strings"
	"time"

	applicationinteraction "github.com/inumoroha/video-feed/internal/application/interaction"
	"github.com/inumoroha/video-feed/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 封装互动、视频和曝光事件的发布与消费通道。
type RabbitMQ struct {
	conn    *amqp.Connection
	publish *amqp.Channel
	consume *amqp.Channel
	cfg     config.RabbitMQConfig
}

// NewRabbitMQ 连接 RabbitMQ 并声明互动、视频和曝光交换机、队列及绑定；初始化失败时关闭已创建资源。
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

// ensureTopology 声明互动、视频与曝光持久化交换机和队列，并绑定各自路由键。
func (r *RabbitMQ) ensureTopology() error {
	if err := r.publish.ExchangeDeclare(r.cfg.InteractionExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := r.publish.QueueDeclare(r.cfg.ActionChangedQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := r.publish.QueueBind(r.cfg.ActionChangedQueue, r.cfg.ActionChangedRouting, r.cfg.InteractionExchange, false, nil); err != nil {
		return err
	}
	if err := r.publish.ExchangeDeclare(r.cfg.VideoExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := r.publish.QueueDeclare(r.cfg.VideoPublishedQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := r.publish.QueueBind(r.cfg.VideoPublishedQueue, r.cfg.VideoPublishedRouting, r.cfg.VideoExchange, false, nil); err != nil {
		return err
	}
	if err := r.publish.ExchangeDeclare(r.cfg.ExposureExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := r.publish.QueueDeclare(r.cfg.ViewEventRecordedQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return r.publish.QueueBind(r.cfg.ViewEventRecordedQueue, r.cfg.ViewEventRecordedRouting, r.cfg.ExposureExchange, false, nil)
}

// normalizeConfig 保留自定义拓扑名称并补齐空白默认值。
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
	if strings.TrimSpace(c.VideoExchange) == "" {
		c.VideoExchange = defaultVideoExchange
	}
	if strings.TrimSpace(c.VideoPublishedQueue) == "" {
		c.VideoPublishedQueue = defaultVideoPublishedQueue
	}
	if strings.TrimSpace(c.VideoPublishedRouting) == "" {
		c.VideoPublishedRouting = defaultVideoPublishedRouting
	}
	c.ExposureExchange = strings.TrimSpace(c.ExposureExchange)
	c.ViewEventRecordedQueue = strings.TrimSpace(c.ViewEventRecordedQueue)
	c.ViewEventRecordedRouting = strings.TrimSpace(c.ViewEventRecordedRouting)
	if c.ExposureExchange == "" {
		c.ExposureExchange = defaultExposureExchange
	}
	if c.ViewEventRecordedQueue == "" {
		c.ViewEventRecordedQueue = defaultViewEventRecordedQueue
	}
	if c.ViewEventRecordedRouting == "" {
		c.ViewEventRecordedRouting = defaultViewEventRecordedRouting
	}
	return c
}

// PublishVideoPublished 发布持久化视频消息，保留事件编号和发送时间。
func (r *RabbitMQ) PublishVideoPublished(ctx context.Context, event *applicationvideo.PublishedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publish.PublishWithContext(
		ctx,
		r.cfg.VideoExchange,
		r.cfg.VideoPublishedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

// ConsumeVideoPublished 消费视频事件；无效消息丢弃、处理失败重新入队、成功逐条确认。
func (r *RabbitMQ) ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	deliveries, err := r.consume.ConsumeWithContext(
		ctx,
		r.cfg.VideoPublishedQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for delivery := range deliveries {
			var event applicationvideo.PublishedEvent
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, &event); err != nil {
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		}
	}()
	return nil
}

const defaultVideoExchange = "videofeed.video"
const defaultVideoPublishedQueue = "videofeed.video.published"
const defaultVideoPublishedRouting = "video.published"

// PublishViewEventRecorded 发布持久化画像事件，保留消息编号、时间和 JSON 内容；空事件不发送。
func (r *RabbitMQ) PublishViewEventRecorded(ctx context.Context, event *applicationexposure.ViewEventRecordedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publish.PublishWithContext(
		ctx,
		r.cfg.ExposureExchange,
		r.cfg.ViewEventRecordedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

// ConsumeViewEventRecorded 启动画像事件消费；成功确认、无效 JSON 丢弃、处理失败重新入队，上下文取消结束消费。
func (r *RabbitMQ) ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) error {
	deliveries, err := r.consume.ConsumeWithContext(
		ctx,
		r.cfg.ViewEventRecordedQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for delivery := range deliveries {
			handleViewEventDelivery(ctx, delivery, handler)
		}
	}()
	return nil
}

const defaultExposureExchange = "videofeed.exposure"
const defaultViewEventRecordedQueue = "videofeed.exposure.view_event_recorded"
const defaultViewEventRecordedRouting = "exposure.view_event_recorded"

// handleViewEventDelivery 解析单条画像消息，成功确认、格式错误丢弃、业务失败重新入队；确认错误与原协议一致不再重试。
func handleViewEventDelivery(ctx context.Context, delivery amqp.Delivery, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) {
	var event applicationexposure.ViewEventRecordedEvent
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		_ = delivery.Nack(false, false)
		return
	}
	if err := handler(ctx, &event); err != nil {
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}
