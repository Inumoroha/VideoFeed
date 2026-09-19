package mq

import (
	"context"
	"errors"
	"testing"

	applicationexposure "github.com/inumoroha/video-feed/internal/application/exposure"
	"github.com/inumoroha/video-feed/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// deliveryAcks 记录画像消息的确认与拒绝参数。
type deliveryAcks struct {
	ack, nack         int
	requeue, multiple bool
	tag               uint64
}

// Ack 记录单条消息确认参数。
func (a *deliveryAcks) Ack(tag uint64, multiple bool) error {
	a.ack++
	a.tag = tag
	a.multiple = multiple
	return nil
}

// Nack 记录单条消息拒绝及重新入队参数。
func (a *deliveryAcks) Nack(tag uint64, multiple, requeue bool) error {
	a.nack++
	a.tag = tag
	a.multiple = multiple
	a.requeue = requeue
	return nil
}

// Reject 对测试中意外使用的拒绝接口返回错误。
func (a *deliveryAcks) Reject(uint64, bool) error { return errors.New("unexpected Reject") }

// TestExposureDeliveryAcknowledgements 验证消息解码、失败重入队后成功确认，以及毒消息丢弃。
func TestExposureDeliveryAcknowledgements(t *testing.T) {
	ack := &deliveryAcks{}
	calls := 0
	handler := func(ctx context.Context, event *applicationexposure.ViewEventRecordedEvent) error {
		calls++
		if event.EventID != "event-1" || event.ViewEventID != 9 {
			t.Fatal(event)
		}
		if calls == 1 {
			return errors.New("retry")
		}
		return nil
	}
	delivery := amqp.Delivery{Acknowledger: ack, DeliveryTag: 7, Body: []byte(`{"event_id":"event-1","view_event_id":9}`)}
	handleViewEventDelivery(context.Background(), delivery, handler)
	if ack.nack != 1 || !ack.requeue || ack.multiple || ack.tag != 7 || ack.ack != 0 {
		t.Fatal(ack)
	}
	handleViewEventDelivery(context.Background(), delivery, handler)
	if calls != 2 || ack.ack != 1 || ack.nack != 1 {
		t.Fatal(ack)
	}
	delivery.Body = []byte("invalid json")
	handleViewEventDelivery(context.Background(), delivery, handler)
	if calls != 2 || ack.nack != 2 || ack.requeue || ack.multiple {
		t.Fatal(ack)
	}
}

// TestExposureQueueConfiguration 验证曝光拓扑的默认值、定制值清洗与空事件发布行为。
func TestExposureQueueConfiguration(t *testing.T) {
	defaults := normalizeConfig(config.RabbitMQConfig{})
	if defaults.ExposureExchange != "videofeed.exposure" || defaults.ViewEventRecordedQueue != "videofeed.exposure.view_event_recorded" || defaults.ViewEventRecordedRouting != "exposure.view_event_recorded" {
		t.Fatal(defaults)
	}
	custom := normalizeConfig(config.RabbitMQConfig{ExposureExchange: " custom.exposure ", ViewEventRecordedQueue: " custom.views ", ViewEventRecordedRouting: " custom.route "})
	if custom.ExposureExchange != "custom.exposure" || custom.ViewEventRecordedQueue != "custom.views" || custom.ViewEventRecordedRouting != "custom.route" {
		t.Fatal(custom)
	}
	if err := (&RabbitMQ{}).PublishViewEventRecorded(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
