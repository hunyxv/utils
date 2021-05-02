package subscribepublish

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPublish(t *testing.T) {
	sp := NewSubscribePublish(WithLogger(zap.NewExample()))

	sp.SubscribeTopic("test", func(v interface{}) { t.Log(v) })
	sp.SubscribeTopic("test", func(v interface{}) { t.Log(v) })
	ok := sp.Publish(Event{Topic: "test", Value: "12345"}, 0)

	t.Log(ok)
	time.Sleep(time.Second)
	sp.Stop()
}

func TestCancelSubscribe(t *testing.T) {
	sp := NewSubscribePublish(WithLogger(zap.NewExample()))
	defer sp.Stop()

	sub1 := sp.SubscribeTopic("test", func(v interface{}) { t.Log(1, v) })
	sp.SubscribeTopic("test", func(v interface{}) { t.Log(2, v) })
	if ok := sp.Publish(Event{Topic: "test", Value: "12345"}, 0); !ok {
		t.Fail()
	}
	time.Sleep(time.Second)
	sp.CancelSubscribe(sub1)
	if ok := sp.Publish(Event{Topic: "test", Value: "12345"}, 0); !ok {
		t.Fail()
	}

	time.Sleep(time.Second * 2)
}

func TestPublishTimeout(t *testing.T) {
	sp := NewSubscribePublish(WithLogger(zap.NewExample()), WithWorkPoolSize(1), WithQueueSize(0))
	defer sp.Stop()

	sp.SubscribeTopic("test", func(v interface{}) { time.Sleep(time.Second * 3); t.Log(1, v) })
	sp.SubscribeTopic("test", func(v interface{}) { time.Sleep(time.Second * 3); t.Log(2, v) })

	sp.Publish(Event{Topic: "test", Value: "12345"}, time.Second*2)
	sp.Publish(Event{Topic: "test", Value: "54321"}, time.Second*2)

	time.Sleep(time.Second * 5)
}

func TestPublishClosed(t *testing.T) {
	sp := NewSubscribePublish(WithLogger(zap.NewExample()))

	sp.SubscribeTopic("test", func(v interface{}) { t.Log(1, v) })
	sp.SubscribeTopic("test", func(v interface{}) { t.Log(2, v) })

	sp.Publish(Event{Topic: "test", Value: "12345"}, 0)
	time.Sleep(500 * time.Millisecond)
	sp.Stop()
	if ok := sp.Publish(Event{Topic: "test", Value: "54321"}, 0); ok {
		t.Failed()
	}
	time.Sleep(2 * time.Second)
}
