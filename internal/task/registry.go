package task

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

// Register registers a cancellable task by id and returns its Done channel.
func Register(id string, cancel context.CancelFunc) chan struct{} {
	task := &Task{
		Cancel: cancel,
		Done:   make(chan struct{}),
	}

	taskMu.Lock()
	tasks[id] = task
	taskMu.Unlock()

	return task.Done
}

// Finish marks the task done and removes it from registry.
func Finish(id string) {
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

// Cancel cancels the task. Returns false when task id does not exist.
func Cancel(id string) bool {
	taskMu.Lock()
	task, ok := tasks[id]
	taskMu.Unlock()

	if !ok {
		return false
	}
	task.Cancel()
	return true
}
