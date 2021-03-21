package timer

import (
	"errors"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

var id uint64

var (
	ErrClose error = errors.New("The WheelTimer has stopped")
)

// Logger is used for logging formatted messages.
type Logger interface {
	// Printf must have the same semantics as log.Printf.
	Printf(format string, args ...interface{})
}

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
	TicksPerWheel int
	// 同时在运行的定时任务数量
	WorkPoolSize int
	// 清除pool中过期的 work 任务
	WorkTimeout time.Duration
	Logger      Logger
}

func WithTickDuration(tickDuration time.Duration) Option {
	return func(opts *Options) {
		opts.TickDuration = tickDuration
	}
}

func WithTicksPerWheel(ticksPerWheel int) Option {
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

func WithLogger(logger Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}

type Timeout struct {
	id     uint64
	task   TimerTask
	round  int
	bucket int
}

func (t *Timeout) isTime() bool {
	t.round--
	return t.round <= 0
}

// func (t *Timeout) cancel() {
// 	return t.task.Cancel()
// }

type TimingWheel struct {
	no    int
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
	ticksPerWheel int
	workPool      *ants.Pool
	newTasksQ     map[int][]*Timeout
	cancelTasksQ  sync.Map
	timingWheel   *TimingWheel
	tick          int
	logger        Logger

	mux   *sync.Mutex
	close bool
	ch    chan struct{}
}

func NewHashedWheelTimer(options ...Option) (*HashedWheelTimer, error) {
	opts := &Options{
		TickDuration:  100 * time.Millisecond,
		TicksPerWheel: 512,
		WorkPoolSize:  500,
		WorkTimeout:   time.Duration(10) * time.Second,
		Logger:        Logger(log.New(os.Stderr, "", log.LstdFlags)),
	}

	for _, option := range options {
		option(opts)
	}

	workPool, err := ants.NewPool(opts.WorkPoolSize, ants.WithExpiryDuration(opts.WorkTimeout))
	if err != nil {
		return nil, err
	}

	timingWheel := new(TimingWheel)
	taskQ := make(map[int][]*Timeout)

	for i := 0; i < opts.TicksPerWheel; i++ {
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

func (hwt *HashedWheelTimer) Submit(after time.Duration, task TimerTask) (timerid uint64, err error) {
	if hwt.close {
		return 0, ErrClose
	}

	timerid = atomic.AddUint64(&id, 1)
	hwt.mux.Lock()
	defer hwt.mux.Unlock()

	tmp := int(after/hwt.tickDuration) + hwt.tick
	timeout := &Timeout{
		id:     timerid,
		task:   task,
		round:  tmp / hwt.ticksPerWheel,
		bucket: tmp % hwt.ticksPerWheel,
	}

	hwt.newTasksQ[timeout.bucket] = append(hwt.newTasksQ[timeout.bucket], timeout)
	if len(hwt.newTasksQ[timeout.bucket]) > 1000 {
		hwt.logger.Printf("WARNING: the task queue length is greater than 1000")
	}
	return
}

func (hwt *HashedWheelTimer) Cancel(timerid uint64) {
	hwt.cancelTasksQ.Store(timerid, struct{}{})
}

func (hwt *HashedWheelTimer) Start() {
	ticker := time.NewTicker(hwt.tickDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
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
					continue
				}
				if timerTask.isTime() {
					hwt.timingWheel.tasks[i] = hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1]
					hwt.timingWheel.tasks[len(hwt.timingWheel.tasks)-1] = nil
					hwt.timingWheel.tasks = hwt.timingWheel.tasks[:len(hwt.timingWheel.tasks)-1]
					if err := hwt.workPool.Submit(func() {
						defer func() {
							if r := recover(); r != nil {
								strerr := r.(string)
								hwt.logger.Printf("[ERROR]:[timeTask]: %s", strerr)
								timerTask.task.Exception(errors.New(strerr))
							}
						}()
						timerTask.task.Run()
					}); err != nil {
						hwt.logger.Printf("[ERROR]:[PoolSubmit]: %s", err.Error())
					}
				}
			}
			hwt.timingWheel = hwt.timingWheel.next
			hwt.tick = hwt.timingWheel.no
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
