package keylock

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// singlelock 单个锁
type singlelock struct {
	flag    uint32
	cleared uint32
	t       int64
}

func (sl *singlelock) lock() bool {
	for !atomic.CompareAndSwapUint32(&(sl.flag), 0, 1) {
		// 此锁已经标记为清除，则加锁失败
		if atomic.LoadUint32(&(sl.cleared)) == 1 {
			return false
		}
		runtime.Gosched()
	}
	return true
}

func (sl *singlelock) unlock() {
	atomic.StoreUint32(&(sl.flag), 0)
}

// mark 标记此锁已经清除
func (sl *singlelock) mark() bool {
	// 先加锁然后标记已清除
	if atomic.CompareAndSwapUint32(&(sl.flag), 0, 1) {
		atomic.StoreUint32(&sl.cleared, 1)
		return true
	}
	// 标记失败，说明此锁已加锁正在被使用
	return false
}

// KeyLock 细粒度锁
type KeyLock struct {
	m       map[string]*singlelock
	ctx     context.Context
	rwmutex sync.RWMutex
}

// NewKeyLock .
func NewKeyLock(ctx context.Context) *KeyLock {
	kl := &KeyLock{
		m:   make(map[string]*singlelock, 100),
		ctx: ctx,
	}

	go func() {
		time.Sleep(5 * time.Second)
		ticker := time.NewTicker(time.Minute * 10)
		defer ticker.Stop()
		for {
			select {
			case <-kl.ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UnixNano()
				kl.rwmutex.Lock()
				for k, l := range kl.m {
					if now-l.t > int64(10*time.Minute) {
						// 标记失败，跳过清除操作
						if !l.mark() {
							continue
						}
						// 清除闲置的锁
						delete(kl.m, k)
					}
				}
				kl.rwmutex.Unlock()
			}
		}
	}()
	return kl
}

// Lock 根据 key 加锁
func (kl *KeyLock) Lock(key string) {
	kl.rwmutex.RLock()
	if single, ok := kl.m[key]; ok {
		kl.rwmutex.RUnlock()
		if ok := single.lock(); !ok {
			// 此锁已被清除，重新获取
			kl.Lock(key)
		}
		return
	}

	// key 的锁不存在，则生成并加锁然后保存
	kl.rwmutex.RUnlock()
	kl.rwmutex.Lock()
	defer kl.rwmutex.Unlock()
	if single, ok := kl.m[key]; ok {
		single.lock()
		return
	}

	single := &singlelock{t: time.Now().UnixNano()}
	single.lock()
	kl.m[key] = single
}

// Unlock 释放锁
func (kl *KeyLock) Unlock(key string) {
	kl.rwmutex.RLock()
	defer kl.rwmutex.RUnlock()
	signle, ok := kl.m[key]
	if !ok {
		return
	}
	signle.unlock()
}
