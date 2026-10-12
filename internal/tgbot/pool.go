package tgbot

import (
	"log"
	"sync"
)

type UpdateWorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

func NewWorkerPool(workers, queueSize int) *UpdateWorkerPool {
	p := &UpdateWorkerPool{
		tasks: make(chan func(), queueSize),
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer p.wg.Done()
			for task := range p.tasks {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[WorkerPool] panic: %v", r)
						}
					}()
					task()
				}()
			}
		}()
	}
	return p
}

func (p *UpdateWorkerPool) Submit(task func()) bool {
	select {
	case p.tasks <- task:
		return true
	default:
		// 队列已满，丢弃或告警，防止 goroutine 无界泄露打满 SQLite
		log.Printf("[WorkerPool] 任务队列满，丢弃或延迟处理")
		return false
	}
}
