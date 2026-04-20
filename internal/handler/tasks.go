package handler

import (
	"context"
	"sync"
)

type Task struct {
	Cancel context.CancelFunc
	Done   chan struct{}
}

var (
	taskMu sync.Mutex
	tasks  = make(map[string]*Task)
)

// RegisterTask 注册一个可取消的流式任务，返回其 Done 通道（任务完成时会被关闭）。
func RegisterTask(id string, cancel context.CancelFunc) chan struct{} {
	task := &Task{
		Cancel: cancel,
		Done:   make(chan struct{}),
	}

	taskMu.Lock()
	tasks[id] = task
	taskMu.Unlock()

	return task.Done
}

// FinishTask 标记任务完成并清理，关闭 Done 通道。
func FinishTask(id string) {
	taskMu.Lock()
	task, ok := tasks[id]
	if ok {
		delete(tasks, id)
	}
	taskMu.Unlock()
	if ok {
		close(task.Done)
	}
}

// CancelTask 取消指定任务；若不存在返回 false。
func CancelTask(id string) bool {
	taskMu.Lock()
	task, ok := tasks[id]
	taskMu.Unlock()

	if !ok {
		return false
	}
	task.Cancel()
	return true
}
