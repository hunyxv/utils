package heaptimer

import (
	"log"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimerWheel(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	tw.AddTask(5*time.Second, func(t time.Time) {
		log.Println(t)
	})
	timer, err := NewTimer(time.Now().Add(3*time.Second), func(t time.Time) {
		log.Println(t)
	})
	tw.AddTimer(timer)

	time.Sleep(6 * time.Second)
}

func TestTimerWheelReset(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	timer, err := tw.AddTask(5*time.Second, func(t time.Time) {
		log.Println(t)
	})

	if err != nil {
		t.Fatal(err)
	}
	timer.ResetWithTime(time.Now().Add(10 * time.Second))

	time.Sleep(11 * time.Second)
}

func TestTimerWheelCancel(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	timer, err := tw.AddTask(5*time.Second, func(t time.Time) {
		log.Println(t)
	})

	if err != nil {
		t.Fatal(err)
	}

	timer.Cancel()

	time.Sleep(6 * time.Second)
}

func TestTimerWheelRepeated(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	timer, err := tw.AddTask(1*time.Second, func(t time.Time) {
		log.Println(t)
	})

	if err != nil {
		t.Fatal(err)
	}

	timer.SetRepeated(true)

	time.Sleep(20 * time.Second)
	timer.Cancel()
	time.Sleep(3 * time.Second)
}

func TestTimerWheelCrond(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	_, err = tw.AddTaskWithCrond("*/1 * * * * * *", func(t time.Time) {
		log.Println(t)
	})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Second)
}

func TestTimerWheelBatch(t *testing.T) {
	tw, err := NewTimerWheel()
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Stop()

	var count int32
	var count2 int32
	for j := 0; j < 10; j++ {
		go func() {
			random := rand.New(rand.NewSource(time.Now().UnixNano()))
			for i := 0; i < 10000; i++ {
				s := time.Duration(random.Intn(30))
				m := time.Duration(random.Intn(1000)) + 100
				atomic.AddInt32(&count2, 1)
				timer, err := tw.AddTask(s*time.Second+m*time.Millisecond, func(t time.Time) {
					atomic.AddInt32(&count, 1)
					//c := atomic.AddInt32(&count, 1)
					//log.Println(t, c)
				})
				if err != nil {
					log.Fatal(err)
				}
				timer.SetOverdueAlsoExec(false)
				timer.SetTimerErrors(50*int64(time.Millisecond), -60*int64(time.Millisecond))
			}
		}()
	}

	time.Sleep(35 * time.Second)
	_, err = tw.heap.Pop()
	if err == nil {
		t.Log("??????")
	}
	t.Log(count, count2)
}
