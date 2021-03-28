package timer

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hunyxv/utils/spinlock"
	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
)

var id uint64

var (
	ErrClose error = errors.New("the WheelTimer has stopped")
)

type TimerTask interface {
	// 执行 task
	Run() error
	// 处理异常
	Exception(error)
	// 取消时是否需要回调
	NeedCancel() bool
	// 取消时执行的函数
	Cancel()
}

type Option func(opts *Options)

type Options struct {
	// 一个 bucket 代表的时间，默认为 100ms
	TickDuration time.Duration
	// 一轮含有多少个 bucket ，默认为 512 个
	TicksPerWheel int32
	// 同时在运行的定时任务数量
	WorkPoolSize int
	// 清除pool中过期的 work 任务
	WorkTimeout time.Duration
	Logger      *zap.Logger
}

func WithTickDuration(tickDuration time.Duration) Option {
	return func(opts *Options) {
		opts.TickDuration = tickDuration
	}
}

func WithTicksPerWheel(ticksPerWheel int32) Option {
	return func(opts *Options) {
		opts.TicksPerWheel = ticksPerWheel
	}
}

func WithWorkPoolSize(poolSize int) Option {
	return func(opts *Options) {
		opts.WorkPoolSize = poolSize
	}
}

func WithWorkTimeout(t time.Duration) Option {
	return func(opts *Options) {
		opts.WorkTimeout = t
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}

type Timeouter interface {
	isTime() bool
	Status() TimeoutStatus
	ID() uint64
}

type TimeoutStatus int

const (
	_                     = iota
	Waiting TimeoutStatus = iota
	Running
	Cancel
	Done
	Panic
)

var _ Timeouter = (*Timeout)(nil)

type Timeout struct {
	id       uint64
	task     TimerTask
	round    int32
	bucket   int32
	spinLock sync.Locker
	status   TimeoutStatus
}

func newTimeout(task TimerTask, round, bucket int32) *Timeout {
	return &Timeout{
		id:       atomic.AddUint64(&id, 1),
		task:     task,
		round:    round,
		bucket:   bucket,
		spinLock: spinlock.NewSpinLock(),
	}
}

func (t *Timeout) isTime() bool {
	t.round--
	return t.round < 0
}

func (t *Timeout) Status() TimeoutStatus {
	t.spinLock.Lock()
	defer t.spinLock.Unlock()
	return t.status
}

func (t *Timeout) setStatus(status TimeoutStatus) {
	t.spinLock.Lock()
	t.status = status
	t.spinLock.Unlock()
}

func (t *Timeout) ID() uint64 {
	return t.id
}

type TimingWheel struct {
	no    int32
	tasks []*Timeout
	next  *TimingWheel
}

func insertCircleNode(head *TimingWheel, newNode *TimingWheel) {
	for head.next == nil {
		head.no = newNode.no
		head.tasks = newNode.tasks
		head.next = head
		return
	}

	temp := head
	for temp.next != head {
		temp = temp.next
	}

	temp.next = newNode
	newNode.next = head
}

type HashedWheelTimer struct {
	tickDuration  time.Duration
	ticksPerWheel int32
	workPool      *ants.Pool
	newTasksQ     map[int32][]*Timeout
	cancelTasksQ  sync.Map
	timingWheel   *TimingWheel
	tick          int32
	logger        *zap.Logger

	mux *sync.Mutex

	close bool
	ch    chan struct{}
}

func NewHashedWheelTimer(options ...Option) (*HashedWheelTimer, error) {
	opts := &Options{
		TickDuration:  100 * time.Millisecond,
		TicksPerWheel: 512,
		WorkPoolSize:  500,
		WorkTimeout:   time.Duration(10) * time.Second,
	}

	for _, option := range options {
		option(opts)
	}

	workPool, err := ants.NewPool(opts.WorkPoolSize, ants.WithExpiryDuration(opts.WorkTimeout))
	if err != nil {
		return nil, err
	}

	timingWheel := new(TimingWheel)
	taskQ := make(map[int32][]*Timeout)

	for i := int32(0); i < opts.TicksPerWheel; i++ {
		insertCircleNode(timingWheel, &TimingWheel{
			no:    i,
			tasks: make([]*Timeout, 0),
		})

		taskQ[i] = make([]*Timeout, 0)
	}

	return &HashedWheelTimer{
		tickDuration:  opts.TickDuration,
		ticksPerWheel: opts.TicksPerWheel,
		workPool:      workPool,
		newTasksQ:     taskQ,
		cancelTasksQ:  sync.Map{},
		timingWheel:   timingWheel,
		tick:          0,
		logger:        opts.Logger,
		mux:           new(sync.Mutex),
		ch:            make(chan struct{}),
	}, nil
}

func (hwt *HashedWheelTimer) Submit(after time.Duration, task TimerTask) (Timeouter, error) {
	if hwt.close {
		return nil, ErrClose
	}

	tick := atomic.LoadInt32(&(hwt.tick))
	tmp := int32(int(after/hwt.tickDuration) + int(tick))
	timeout := newTimeout(task, tmp/hwt.ticksPerWheel, tmp%hwt.ticksPerWheel)
	hwt.mux.Lock()
	hwt.newTasksQ[timeout.bucket] = append(hwt.newTasksQ[timeout.bucket], timeout)
	defer hwt.mux.Unlock()
	timeout.status = Waiting
	if hwt.logger != nil && len(hwt.newTasksQ[timeout.bucket]) > 10000 {
		hwt.logger.Warn("the task queue length is greater than 1000")
	}
	return timeout, nil
}

func (hwt *HashedWheelTimer) ExecuteAt(t time.Time, task TimerTask) (Timeouter, error) {
	return hwt.Submit(time.Until(t), task)
}

func (hwt *HashedWheelTimer) Cancel(timeout Timeouter) {
	if timeout.Status() == Waiting {
		if timeout.Status() == Waiting {
			hwt.cancelTasksQ.Store(timeout.ID(), struct{}{})
		}
	}
}

func (hwt *HashedWheelTimer) Start() {
	ticker := time.NewTicker(hwt.tickDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			start := time.Now()
			hwt.mux.Lock()
			hwt.timingWheel.tasks = append(hwt.timingWheel.tasks, hwt.newTasksQ[hwt.tick]...)
			for i := range hwt.newTasksQ[hwt.tick] {
				hwt.newTasksQ[hwt.tick][i] = nil
			}
			hwt.newTasksQ[hwt.tick] = hwt.newTasksQ[hwt.tick][:0]
			hwt.mux.Unlock()

			for i := 0; i < len(hwt.timingWheel.tasks); {
				timerTask := hwt.timingWheel.tasks[i]

				if _, loaded := hwt.cancelTasksQ.LoadAndDelete(timerTask.id); loaded {
					hwt.timingWheel.tasks[i] = hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1]
					hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1] = nil
					hwt.timingWheel.tasks = hwt.timingWheel.tasks[:len(hwt.timingWheel.tasks)-1]

					if timerTask.task.NeedCancel() {
						hwt.workPool.Submit(timerTask.task.Cancel)
					}
					timerTask.setStatus(Cancel)
					continue
				}
				if timerTask.isTime() {
					timerTask.setStatus(Running)
					hwt.timingWheel.tasks[i] = hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1]
					hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1] = nil
					hwt.timingWheel.tasks = hwt.timingWheel.tasks[:len(hwt.timingWheel.tasks)-1]
					if err := hwt.workPool.Submit(func() {
						defer func() {
							if r := recover(); r != nil {
								strerr := r.(string)
								timerTask.task.Exception(errors.New(strerr))
								timerTask.setStatus(Panic)
								return
							}
							timerTask.setStatus(Done)
						}()
						timerTask.task.Run()
					}); err != nil {
						if hwt.logger != nil {
							hwt.logger.Error("err: %s", zap.Error(err))
						}
					}
					continue
				}
				i++
			}
			hwt.timingWheel = hwt.timingWheel.next
			atomic.StoreInt32(&(hwt.tick), hwt.timingWheel.no)

			if hwt.logger != nil && time.Since(start) > hwt.tickDuration {
				hwt.logger.Warn("polling for tasks takes longer than tickDuration")
			}
		case <-hwt.ch:
			hwt.close = true
			return
		}
	}
}

func (hwt *HashedWheelTimer) Stop() {
	hwt.ch <- struct{}{}
	hwt.workPool.Release()
}
