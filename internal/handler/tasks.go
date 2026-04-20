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
