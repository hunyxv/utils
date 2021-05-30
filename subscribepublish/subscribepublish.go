package subscribepublish

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hunyxv/utils/spinlock"
	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
)

type handleID uint32

var _HANDLE_ID uint32

func nextID() uint32 {
	return atomic.AddUint32(&_HANDLE_ID, 1)
}

type logger struct {
	logger *zap.Logger
}

func (l *logger) Printf(format string, args ...interface{}) {
	if l.logger != nil {
		l.logger.Error(fmt.Sprintf("SubscribePublish|"+format, args...))
	}
}

type Event struct {
	Topic string      // 主题
	Value interface{} // 内容
}

type Handler interface {
	handle(interface{})
	cancel()
}

var _ Handler = HandlerFunc(nil)

type HandlerFunc func(interface{})

func (h HandlerFunc) handle(v interface{}) { h(v) }
func (HandlerFunc) cancel()                {}

var _ Handler = (*handlerChan)(nil)

type handlerChan struct {
	c     chan Event
	topic string
}

func newHandlerChan(topic string, size int) *handlerChan {
	if size < 0 {
		size = 0
	}
	handle := &handlerChan{
		c:     make(chan Event, size),
		topic: topic,
	}
	return handle
}

func (h *handlerChan) handle(v interface{}) {
	event := Event{Topic: h.topic, Value: v}
	h.c <- event
}

func (h *handlerChan) cancel() {
	close(h.c)
}

type Option func(opts *option)

type option struct {
	QueueSize int
	// 同时在运行的定时任务数量
	WorkPoolSize int
	// 清除pool中过期的 work 任务
	WorkTimeout time.Duration
	Logger      *zap.Logger
}

func WithQueueSize(size int) Option {
	return func(opts *option) {
		opts.QueueSize = size
	}
}

func WithWorkPoolSize(poolSize int) Option {
	return func(opts *option) {
		opts.WorkPoolSize = poolSize
	}
}

func WithWorkTimeout(t time.Duration) Option {
	return func(opts *option) {
		opts.WorkTimeout = t
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(opts *option) {
		opts.Logger = logger
	}
}

type SubscribePublish struct {
	eventChan    chan Event
	topicHandler map[string][]uint32
	handleList   map[uint32]Handler
	workPool     *ants.Pool
	opts         *option
	closed       bool

	mux    sync.Locker
	ctx    context.Context
	cancel context.CancelFunc
}

func NewSubscribePublish(options ...Option) *SubscribePublish {
	opts := &option{
		QueueSize:    1,
		WorkPoolSize: 500,
	}

	for _, f := range options {
		f(opts)
	}

	workPool, err := ants.NewPool(
		opts.WorkPoolSize,
		ants.WithExpiryDuration(opts.WorkTimeout),
		ants.WithLogger(&logger{opts.Logger}),
	)
	if err != nil {
		opts.Logger.Error("NewSubscribePublish|Fail|%s", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	sp := &SubscribePublish{
		eventChan:    make(chan Event, opts.QueueSize),
		topicHandler: make(map[string][]uint32),
		handleList:   make(map[uint32]Handler),
		workPool:     workPool,
		opts:         opts,

		mux:    spinlock.NewSpinLock(),
		ctx:    ctx,
		cancel: cancel,
	}

	// start
	go func() {
		for {
			select {
			case <-sp.ctx.Done():
				sp.opts.Logger.Info("NewSubscribePublish|Stop")
				return
			case event := <-sp.eventChan:
				if hs, ok := sp.topicHandler[event.Topic]; ok {
					sp.mux.Lock()
					for _, hid := range hs {
						value := event.Value
						handler := sp.handleList[hid]
						sp.workPool.Submit(
							func() {
								handler.handle(value)
							},
						)
					}
					sp.mux.Unlock()
				}
			}
		}
	}()

	return sp
}

// Publish 发布 event， timeout 若为0则阻塞至消息发出
func (sp *SubscribePublish) Publish(event Event, timeout time.Duration) (ok bool) {
	defer func() {
		if e := recover(); e != nil {
			ok = false
		}
	}()

	if sp.closed {
		return false
	}

	_, ok = sp.topicHandler[event.Topic]
	if ok {
		if timeout <= 0 {
			sp.eventChan <- event
		} else {
			timer := time.NewTimer(timeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				sp.opts.Logger.Warn("SubscribePublish|Publish|timeout")
				return false
			case sp.eventChan <- event:
			}
		}
	}
	return
}

// SubscribeTopic 订阅 Topic
func (sp *SubscribePublish) SubscribeTopic(topic string, handler HandlerFunc) handleID {
	hid := nextID()
	sp.mux.Lock()
	defer sp.mux.Unlock()

	sp.topicHandler[topic] = append(sp.topicHandler[topic], hid)
	sp.handleList[hid] = handler
	return handleID(hid)
}

// SubscribeTopicWithChan 通过 chan 订阅 topic
func (sp *SubscribePublish) SubscribeTopicWithChan(topic string, size int) (<-chan Event, handleID) {
	h := newHandlerChan(topic, size)
	hid := nextID()

	sp.mux.Lock()
	defer sp.mux.Unlock()
	sp.topicHandler[topic] = append(sp.topicHandler[topic], hid)
	sp.handleList[hid] = h
	return h.c, handleID(hid)
}

func (sp *SubscribePublish) CancelSubscribe(handleID handleID) {
	hid := uint32(handleID)
	sp.mux.Lock()
	defer sp.mux.Unlock()
	for topic, handles := range sp.topicHandler {
		for i, id := range handles {
			if id == hid {
				handles[i] = handles[len(handles)-1]
				handles = handles[:len(handles)-1]
				sp.topicHandler[topic] = handles
			}
		}
		break
	}
	if handle, ok := sp.handleList[hid]; ok {
		delete(sp.handleList, hid)
		handle.cancel()
	}
}

func (sp *SubscribePublish) Stop() {
	if sp.closed {
		return
	}
	sp.closed = true
	sp.cancel()
	close(sp.eventChan)
	for _, h := range sp.handleList {
		h.cancel()
	}
	sp.workPool.Release()
}
