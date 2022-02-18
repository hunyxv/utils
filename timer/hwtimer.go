package timer

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hunyxv/utils/spinlock"
	"github.com/panjf2000/ants/v2"
)

var (
	// 任务id
	taskID uint64
)

// TimerTaskStatus 定时任务状态
type timerTaskStatus int

const (
	Waiting timerTaskStatus = iota + 1
	Running
	Cancel
	Done
	Panic
)

var TimerTaskStatusMap = map[timerTaskStatus]string{
	Waiting: "waiting",
	Running: "running",
	Cancel:  "cancel",
	Done:    "done",
	Panic:   "panic",
}

// TimerTask 延时任务
type TimerTask interface {
	// TID 任务id
	TID() uint64
	Status() timerTaskStatus
	// Run 执行任务
	Run()
	// Reset 重设过期时间
	Reset() error
	// Cancel 取消任务
	Cancel() error
	// ExpirationTime 到期时间
	ExpirationTime() time.Time
	// isTime 是否到时间了
	isTime() bool
}

var _ (TimerTask) = (*timerTask)(nil)

type timerTask struct {
	id             uint64
	t              func()
	round          uint32 // 轮数
	bucket         uint32 // 所在 bucket
	spinLock       sync.Locker
	status         timerTaskStatus
	expirationTime time.Time
	delayTime      time.Duration
	hwt            *hashedWheelTimer
}

func newTimerTask(task func(), round, bucket uint32, delayTime time.Duration, hwt *hashedWheelTimer) *timerTask {
	expir := time.Now().Add(delayTime)
	return &timerTask{
		id:             atomic.AddUint64(&taskID, 1),
		t:              task,
		round:          round,
		bucket:         bucket,
		spinLock:       spinlock.NewSpinLock(),
		status:         Waiting,
		expirationTime: expir,
		delayTime:      delayTime,
		hwt:            hwt,
	}
}

func (t *timerTask) TID() uint64 {
	return t.id
}

func (t *timerTask) Status() timerTaskStatus {
	t.spinLock.Lock()
	defer t.spinLock.Unlock()
	return t.status
}

func (t *timerTask) setTaskStatus(s timerTaskStatus) error {
	t.spinLock.Lock()
	defer t.spinLock.Unlock()
	if t.status == Waiting {
		switch s {
		case Running, Cancel:
			t.status = s
		default:
			return fmt.Errorf("cannot transition from [%s] to [%s]", TimerTaskStatusMap[t.status], TimerTaskStatusMap[s])
		}
	} else if t.status == Running {
		switch s {
		case Done, Panic:
			t.status = s
		default:
			return fmt.Errorf("cannot transition from [%s] to [%s]", TimerTaskStatusMap[t.status], TimerTaskStatusMap[s])
		}
	}
	return fmt.Errorf("cannot transition from [%s] to [%s]", TimerTaskStatusMap[t.status], TimerTaskStatusMap[s])
}

func (t *timerTask) isTime() bool {
	t.spinLock.Lock()
	defer t.spinLock.Unlock()
	t.round--
	return t.round == 0
}

func (t *timerTask) bucketID() uint32 {
	return t.bucket
}

func (t *timerTask) ExpirationTime() time.Time {
	return t.expirationTime
}

func (t *timerTask) Reset() error {
	t.hwt.cancelTask(t)
	t.status = Waiting
	t.expirationTime = time.Now().Add(t.delayTime)
	t.hwt.submitTask(t)
	return nil
}

func (t *timerTask) Cancel() error {
	t.hwt.cancelTask(t)
	return nil
}

func (t *timerTask) Run() {
	t.setTaskStatus(Running)
	defer func() {
		if e := recover(); e != nil {
			log.Printf("[PANIC] timer task: %v", e)
			t.setTaskStatus(Panic)
		}
	}()

	t.t()
	t.setTaskStatus(Done)
}

// timingWheel 时间轮节点
type timingWheel struct {
	i        uint32
	bucket   []*timerTask
	spinLock sync.Locker
	next     *timingWheel
}

func (w *timingWheel) insertTask(t *timerTask) {
	w.spinLock.Lock()
	defer w.spinLock.Unlock()
	w.bucket = append(w.bucket, t)
	sort.Sort(w)
}

func (w *timingWheel) findTaskByID(tid uint64) *timerTask {
	w.spinLock.Lock()
	defer w.spinLock.Unlock()
	i, j := 0, len(w.bucket)
	for i < j {
		h := int(uint(i+j) >> 1)
		if w.bucket[h].TID() < tid {
			i = h + 1
		} else {
			j = h
		}
	}

	if i >= len(w.bucket) {
		return nil
	}
	return w.bucket[i]
}

func (w *timingWheel) removeTask(tid uint64) *timerTask {
	w.spinLock.Lock()
	defer w.spinLock.Unlock()
	i, j := 0, len(w.bucket)
	for i < j {
		h := int(uint(i+j) >> 1)
		if w.bucket[h].TID() < tid {
			i = h + 1
		} else {
			j = h
		}
	}

	if i < len(w.bucket) {
		tt := w.bucket[i]
		w.bucket = append(w.bucket[:i], w.bucket[i+1:]...)
		return tt
	}
	return nil
}

// Range 遍历 bucket 中所有元素
func (w *timingWheel) Range(f func(t *timerTask)) {
	w.spinLock.Lock()
	defer w.spinLock.Unlock()

	for i := 0; i < len(w.bucket); {
		t := w.bucket[i]
		if t.isTime() {
			w.bucket[i] = w.bucket[len(w.bucket)-1]
			w.bucket[len(w.bucket)-1] = nil
			w.bucket = w.bucket[:len(w.bucket)-1]
			f(t)
			continue
		}
		i++
	}
	sort.Sort(w)
}

func (w *timingWheel) Len() int {
	return len(w.bucket)
}

func (w *timingWheel) Less(i, j int) bool {
	return w.bucket[i].TID() < w.bucket[j].TID()
}

func (w *timingWheel) Swap(i, j int) {
	w.bucket[i], w.bucket[j] = w.bucket[j], w.bucket[i]
}

// insertCircleNode 插入时间轮节点
func insertCircleNode(head *timingWheel, newNode *timingWheel) {
	for head.next == nil {
		head.i = newNode.i
		head.bucket = newNode.bucket
		head.spinLock = newNode.spinLock
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

type hashedWheelTimer struct {
	ctx    context.Context
	cancel context.CancelFunc

	tick          time.Duration // 一个 bucket 的时间跨度/间隔
	ticksPerWheel uint32        // 时间轮 bucket 数量
	gpool         *ants.Pool    // groutine 工作池
	timingWheel   *timingWheel  // 时间轮
	watchHand     uint32        // 表针
}

func NewHashedWheelTimer(pctx context.Context, opts ...Option) (*hashedWheelTimer, error) {
	defaultOpts := &Options{
		TickDuration:  100 * time.Millisecond,
		TicksPerWheel: 512,
	}
	for _, f := range opts {
		f(defaultOpts)
	}
	if defaultOpts.WorkPool == nil {
		pool, err := ants.NewPool(0, ants.WithNonblocking(true))
		if err != nil {
			return nil, err
		}
		defaultOpts.WorkPool = pool
	}

	wheel := new(timingWheel)
	for i := uint32(0); i < defaultOpts.TicksPerWheel; i++ {
		insertCircleNode(wheel, &timingWheel{
			i:        i,
			bucket:   make([]*timerTask, 0),
			spinLock: new(sync.Mutex), //spinlock.NewSpinLock(),
		})
	}

	ctx, cancel := context.WithCancel(pctx)
	return &hashedWheelTimer{
		ctx:    ctx,
		cancel: cancel,

		tick:          defaultOpts.TickDuration,
		ticksPerWheel: defaultOpts.TicksPerWheel,
		gpool:         defaultOpts.WorkPool,
		timingWheel:   wheel,
		watchHand:     0,
	}, nil
}

// Submit 提交延时任务
func (hwt *hashedWheelTimer) Submit(after time.Duration, task func()) TimerTask {
	var tt *timerTask
	watchHand := atomic.LoadUint32(&(hwt.watchHand))
	if after == 0 {
		tt = newTimerTask(task, 1, watchHand, after, hwt)
		hwt.timingWheel.next.insertTask(tt)
		return tt
	}
	totalSpan := uint32(after/hwt.tick) + watchHand

	round := totalSpan/hwt.ticksPerWheel + 1
	bucket := totalSpan % hwt.ticksPerWheel

	node := hwt.timingWheel
	for i := uint32(0); i < hwt.ticksPerWheel; i++ {
		if node.i == bucket {
			tt = newTimerTask(task, round, bucket, after, hwt)
			node.insertTask(tt)
			break
		}
		node = node.next
	}
	return tt
}

// ExecuteAt 在指定时间执行任务
func (hwt *hashedWheelTimer) ExecuteAt(t time.Time, task func()) TimerTask {
	return hwt.Submit(time.Until(t), task)
}

func (hwt *hashedWheelTimer) submitTask(tt *timerTask) {
	watchHand := atomic.LoadUint32(&(hwt.watchHand))
	totalSpan := uint32(tt.delayTime/hwt.tick) + watchHand
	round := totalSpan/hwt.ticksPerWheel + 1
	bucket := totalSpan % hwt.ticksPerWheel
	tt.round = round
	tt.bucket = bucket

	node := hwt.timingWheel
	for i := uint32(0); i < hwt.ticksPerWheel; i++ {
		if node.i == bucket {
			node.insertTask(tt)
			break
		}
		node = node.next
	}
}

func (hwt *hashedWheelTimer) cancelTask(tt *timerTask) bool {
	node := hwt.timingWheel
	for i := uint32(0); i < hwt.ticksPerWheel; i++ {
		if node.i == tt.bucketID() {
			if _tt := node.removeTask(tt.TID()); _tt != nil {
				tt.setTaskStatus(Cancel)
				return true
			}
			return false
		}
		node = node.next
	}
	return false
}

// CancelTaskByID 根据任务ID取消
func (hwt *hashedWheelTimer) CancelTaskByID(tid uint64) bool {
	node := hwt.timingWheel
	for i := uint32(0); i < hwt.ticksPerWheel; i++ {
		if tt := node.removeTask(tid); tt != nil {
			tt.setTaskStatus(Cancel)
			return true
		}
		node = node.next
	}
	return false
}

// FindTaskByID 根据任务id返回任务本体
func (hwt *hashedWheelTimer) FindTaskByID(tid uint64) TimerTask {
	node := hwt.timingWheel
	for i := uint32(0); i < hwt.ticksPerWheel; i++ {
		if tt := node.findTaskByID(tid); tt != nil {
			return tt
		}
		node = node.next
	}
	return nil
}

func (hwt *hashedWheelTimer) runTask(t *timerTask) {
	if err := hwt.gpool.Submit(t.Run); err != nil {
		log.Printf("gpool submit task failed: %v", err)
	}
}

func (hwt *hashedWheelTimer) Start() {
	tick := time.NewTicker(hwt.tick)
	defer tick.Stop()

	for {
		select {
		case <-hwt.ctx.Done():
			return
		case <-tick.C:
			hwt.timingWheel = hwt.timingWheel.next
			atomic.StoreUint32(&hwt.watchHand, hwt.timingWheel.i)
			go hwt.timingWheel.Range(hwt.runTask)
		}
	}
}

func (hwt *hashedWheelTimer) Stop() {
	hwt.cancel()
}
