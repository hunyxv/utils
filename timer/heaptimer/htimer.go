package heaptimer

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/panjf2000/ants/v2"
)

var (
	tid int64

	ErrTimerIsCancelled = errors.New("timer is cancelled")
	ErrTimerTimeout     = errors.New("wait timeout")
)

type Task func(t time.Time)

type Timer struct {
	tid             int64         // 任务id
	task            Task          // 任务函数
	ctime           time.Time     // 创建时间
	expired         int64         // 到期时间（纳秒）
	interval        time.Duration // 时间间隔
	cronetab        *cronexpr.Expression
	repeated        bool  // 是循环任务 默认不是
	overdueAlsoExec bool  // 过期也执行 默认执行
	positiveErrors  int64 // 正误差 默认 50ms
	negativeErrors  int64 // 负误差 默认 -inf

	lock      sync.Mutex
	tw        *TimerWheel
	cancelled bool
}

func NewTimer(expired time.Time, task Task) (*Timer, error) {
	now := time.Now()
	interval := expired.Sub(now)
	if interval < 0 {
		return nil, errors.New("must be a future point in time")
	}

	return &Timer{
		tid:             atomic.AddInt64(&tid, 1),
		task:            task,
		expired:         expired.UnixNano(),
		interval:        interval,
		ctime:           time.Now(),
		overdueAlsoExec: true,
		positiveErrors:  50 * int64(time.Millisecond),
		negativeErrors:  math.MinInt64,
	}, nil
}

// TID 任务id
func (t *Timer) TID() int64 {
	return t.tid
}

// RunningTime 执行的时间
func (t *Timer) RunningTime() time.Time {
	t.lock.Lock()
	_t := time.Unix(0, t.expired)
	t.lock.Unlock()
	return _t
}

// ResetWithTime 重设过期时间
func (t *Timer) ResetWithTime(expired time.Time) error {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return ErrTimerIsCancelled
	}

	t.expired = expired.UnixNano()
	t.lock.Unlock()
	t.tw.ResetTimer(t)
	return nil
}

// Reset 根据上次设置的时间重设
func (t *Timer) Reset() error {
	return t.ResetWithTime(time.Now().Add(t.interval))
}

// SetRepeated 设置/取消循环任务
func (t *Timer) SetRepeated(on_off bool) error {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return ErrTimerIsCancelled
	}

	t.repeated = on_off
	t.lock.Unlock()
	return nil
}

// SetCrond 设置 crond 表达式
func (t *Timer) SetCrond(crond *cronexpr.Expression) error {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return ErrTimerIsCancelled
	}

	t.cronetab = crond
	t.lock.Unlock()
	return nil
}

// SetOverdueAlsoExec 设置任务超时也执行
func (t *Timer) SetOverdueAlsoExec(on_off bool) error {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return ErrTimerIsCancelled
	}

	t.overdueAlsoExec = on_off
	t.lock.Unlock()
	return nil
}

// SetTimerErrors 设置时间误差上下限 默认为[-inf, 100ms]
func (t *Timer) SetTimerErrors(ulimit, llimit int64) error {
	if ulimit < 0 {
		ulimit = int64(time.Millisecond) * 50
	}
	if llimit > 0 {
		llimit = math.MinInt64
	}

	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return ErrTimerIsCancelled
	}

	t.positiveErrors = ulimit
	t.negativeErrors = llimit
	t.lock.Unlock()
	return nil
}

func (t *Timer) setTimerWheel(tw *TimerWheel) {
	t.lock.Lock()
	t.tw = tw
	t.lock.Unlock()
}

func (t *Timer) NextTime(now time.Time) (int64, bool) {
	if t.repeated {
		return now.Add(t.interval).UnixNano(), true
	}

	if t.cronetab != nil {
		return t.cronetab.Next(time.Unix(0, t.expired)).UnixNano(), true
	}
	return 0, false
}

func (t *Timer) istime(now time.Time) (bool, error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.cancelled {
		return false, ErrTimerIsCancelled
	}

	remaining := t.expired - now.UnixNano()
	if remaining > t.positiveErrors {
		return false, nil
	}

	if remaining >= t.negativeErrors || t.overdueAlsoExec {
		return true, nil
	} else {
		return false, ErrTimerTimeout
	}
}

func (t *Timer) timeTask(_t time.Time) func() {
	return func() {
		t.lock.Lock()
		if nextTime, ok := t.NextTime(_t); ok {
			t.expired = nextTime
			t.tw.add(t)
		}
		t.lock.Unlock()

		t.task(_t)
	}
}

func (t *Timer) Cancel() {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return
	}
	t.cancelled = true
	t.tw.removeTimer(t)
	t.lock.Unlock()
}

func (t *Timer) cancel() {
	t.lock.Lock()
	if t.cancelled {
		t.lock.Unlock()
		return
	}
	t.cancelled = true
	t.lock.Unlock()
}

func (t *Timer) Key() interface{} {
	return t.tid
}

func (t *Timer) Value() float64 {
	return float64(t.expired)
}

type TimerWheel struct {
	heap *FibHeap

	ctx      context.Context
	cancel   context.CancelFunc
	workPool *ants.Pool
	logger   Logger
	isclosed int32
}

func NewTimerWheel(opts ...Option) (*TimerWheel, error) {
	pool, err := ants.NewPool(-1)
	if err != nil {
		return nil, err
	}

	defOpt := &options{
		Logger:   &defaultLogger{Logger: log.Default()},
		WorkPool: pool,
	}

	for _, f := range opts {
		f(defOpt)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tw := &TimerWheel{
		heap: NewFibHeap(MinHeap),

		ctx:      ctx,
		cancel:   cancel,
		workPool: defOpt.WorkPool,
		logger:   defOpt.Logger,
	}
	go tw.runloop()
	return tw, nil
}

func (tw *TimerWheel) AddTask(after time.Duration, task Task) (*Timer, error) {
	if atomic.LoadInt32(&tw.isclosed) == 1 {
		return nil, errors.New("TimerWheel is closed")
	}

	now := time.Now()
	timer := &Timer{
		tid:             atomic.AddInt64(&tid, 1),
		task:            task,
		expired:         now.Add(after).UnixNano(),
		interval:        after,
		ctime:           now,
		overdueAlsoExec: true,
		positiveErrors:  50 * int64(time.Millisecond),
		negativeErrors:  math.MinInt64,
		tw:              tw,
	}
	err := tw.heap.Insert(timer)
	if err != nil {
		return nil, err
	}
	return timer, nil
}

func (tw *TimerWheel) AddTaskWithCrond(crond string, task Task) (*Timer, error) {
	if atomic.LoadInt32(&tw.isclosed) == 1 {
		return nil, errors.New("TimerWheel is closed")
	}

	expr, err := cronexpr.Parse(crond)
	if err != nil {
		return nil, err
	}

	nextTime := expr.Next(time.Now())
	timer, err := NewTimer(nextTime, task)
	if err != nil {
		return nil, err
	}
	timer.SetCrond(expr)

	err = tw.AddTimer(timer)
	if err != nil {
		return nil, err
	}

	return timer, nil
}

func (tw *TimerWheel) AddRepeatedTask(after time.Duration, task Task) (*Timer, error) {
	if atomic.LoadInt32(&tw.isclosed) == 1 {
		return nil, errors.New("TimerWheel is closed")
	}

	now := time.Now()
	timer := &Timer{
		tid:             atomic.AddInt64(&tid, 1),
		task:            task,
		expired:         now.Add(after).UnixNano(),
		interval:        after,
		ctime:           now,
		repeated:        true,
		overdueAlsoExec: true,
		positiveErrors:  50 * int64(time.Millisecond),
		negativeErrors:  math.MinInt64,
		tw:              tw,
	}
	err := tw.heap.Insert(timer)
	if err != nil {
		return nil, err
	}
	return timer, nil
}

func (tw *TimerWheel) AddTimer(timer *Timer) error {
	if atomic.LoadInt32(&tw.isclosed) == 1 {
		return errors.New("TimerWheel is closed")
	}

	timer.setTimerWheel(tw)
	return tw.heap.Insert(timer)
}

func (tw *TimerWheel) add(timer *Timer) error {
	if atomic.LoadInt32(&tw.isclosed) == 1 {
		return nil
	}
	return tw.heap.Insert(timer)
}

func (tw *TimerWheel) runTimerTask(t time.Time) {
	val, err := tw.heap.Pop()
	if err != nil {
		return
	}
	tw.workPool.Submit(val.(*Timer).timeTask(t))
}

func (tw *TimerWheel) CancelTimerByID(id int64) {
	v := tw.heap.Delete(id)
	if v == nil {
		tw.logger.Warn("timer could not be found, id: ", id)
		return
	}

	timer := v.(*Timer)
	timer.cancel()
}

func (tw *TimerWheel) removeTimer(key interface{}) {
	tw.heap.Delete(key)
}

func (tw *TimerWheel) ResetTimer(timer *Timer) {
	tw.heap.UpdateValue(timer)
}

func (tw *TimerWheel) runloop() {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-tw.ctx.Done():
			return
		case now := <-tick.C:
			for {
				value, err := tw.heap.Peek()
				if err != nil {
					break
				}

				timer := value.(*Timer)
				istime, err := timer.istime(now)
				if err != nil {
					if err == ErrTimerTimeout {
						tw.logger.Warn("timer wait timeout, id: ", timer.TID(), timer.interval)
					}
					// tw.heap.Delete(timer.Key())
					tw.heap.Pop()
					continue
				}

				if !istime {
					break
				}

				tw.runTimerTask(now)
			}
		}
	}
}

func (tw *TimerWheel) Stop() {
	if !atomic.CompareAndSwapInt32(&tw.isclosed, 0, 1) {
		return
	}
	tw.cancel()
	tw.workPool.Release()
}
