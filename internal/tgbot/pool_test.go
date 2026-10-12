package tgbot

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPoolBoundsConcurrency 回归 ISSUE-03：工作池必须把并发度限制在 workers 以内，
// 而不是每条更新裸起 goroutine 争抢 SQLite 单连接。
func TestPoolBoundsConcurrency(t *testing.T) {
	const workers = 4
	p := NewWorkerPool(workers, 256)

	var active, maxActive, done int64
	var mu sync.Mutex
	for i := 0; i < 64; i++ {
		if !p.Submit(func() {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			atomic.AddInt64(&done, 1)
		}) {
			t.Fatalf("任务 %d 被拒，队列容量 256 不应满", i)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&done) < 64 {
		if time.Now().After(deadline) {
			t.Fatalf("仅完成 %d/64 个任务", atomic.LoadInt64(&done))
		}
		time.Sleep(2 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive > workers {
		t.Fatalf("最大并发 %d 超过 workers=%d", maxActive, workers)
	}
	if maxActive == 0 {
		t.Fatal("没有任何任务执行")
	}
}

// TestPoolRejectsWhenFull 队列有界：满了必须拒绝（返回 false）并告警，而不是无限堆积。
func TestPoolRejectsWhenFull(t *testing.T) {
	const workers, queueSize = 2, 4
	p := NewWorkerPool(workers, queueSize)
	block := make(chan struct{})
	accepted := 0
	for i := 0; i < 100; i++ {
		if p.Submit(func() { <-block }) {
			accepted++
			continue
		}
		break
	}
	// 至多 workers + queueSize 个任务可被接纳；下限为队列容量（worker 可能尚未取走任务）
	if accepted > workers+queueSize {
		t.Fatalf("接纳了 %d 个任务，超过 workers+queue=%d，队列无界", accepted, workers+queueSize)
	}
	if accepted < queueSize {
		t.Fatalf("仅接纳 %d 个任务，少于队列容量 %d", accepted, queueSize)
	}
	close(block)
}

// TestPoolRecoversPanic worker 不得因单个任务 panic 而退出（否则并发度静默下降）。
func TestPoolRecoversPanic(t *testing.T) {
	p := NewWorkerPool(2, 8)
	if !p.Submit(func() { panic("boom") }) {
		t.Fatal("提交 panic 任务失败")
	}
	var ran int64
	done := make(chan struct{})
	if !p.Submit(func() {
		atomic.AddInt64(&ran, 1)
		close(done)
	}) {
		t.Fatal("panic 之后提交失败：worker 可能已退出")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic 后的后续任务未被处理，worker 已死")
	}
	if atomic.LoadInt64(&ran) != 1 {
		t.Fatalf("ran = %d, want 1", ran)
	}
}

// TestCoreSubmitTaskFallback 未注入池时 SubmitTask 退化为异步执行且不报错。
func TestCoreSubmitTaskFallback(t *testing.T) {
	c := &Core{}
	done := make(chan struct{})
	if !c.SubmitTask(func() { close(done) }) {
		t.Fatal("nil Pool 时 SubmitTask 应返回 true")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("nil Pool 时任务未执行")
	}
}

// TestCoreSubmitTaskUsesPool 注入池后必须走池（有界），而不是裸 go func。
func TestCoreSubmitTaskUsesPool(t *testing.T) {
	p := NewWorkerPool(1, 1)
	c := &Core{Pool: p}
	block := make(chan struct{})
	started := make(chan struct{})
	// 第一个任务被唯一 worker 取走并阻塞，确保队列已排空
	if !c.SubmitTask(func() { close(started); <-block }) {
		t.Fatal("第一个任务应被接纳")
	}
	<-started
	// 1 worker + 1 队列：第二个进队列，第三个必须被拒（证明走了池）
	if !c.SubmitTask(func() { <-block }) {
		t.Fatal("第二个任务应被接纳（队列容量 1）")
	}
	if c.SubmitTask(func() { <-block }) {
		t.Fatal("第三个任务应被池拒绝；若成功说明绕过了池")
	}
	close(block)
}
