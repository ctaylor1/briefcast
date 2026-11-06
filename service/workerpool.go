package service

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// SafeGo runs fn in a new goroutine. A panic in fn is logged and ends only fn, so
// background work cannot take the server down with it.
func SafeGo(name string, fn func()) {
	go runSafely(name, fn)
}

// runSafely calls fn and contains any panic. A panic skips ordinary code after the
// point it was raised, so fn must clear any "running" state in a defer.
func runSafely(name string, fn func()) {
	defer func() {
		if value := recover(); value != nil {
			Logger.Errorw("background work panicked", "work", name, "panic", fmt.Sprint(value), "stack", string(debug.Stack()))
		}
	}()
	fn()
}

func boundedWorkerCount(requested int, fallback int, jobCount int) int {
	if fallback <= 0 {
		fallback = 1
	}

	workers := requested
	if workers <= 0 {
		workers = fallback
	}
	if workers <= 0 {
		workers = 1
	}
	if jobCount > 0 && workers > jobCount {
		workers = jobCount
	}
	return workers
}

func runWorkerPool[T any](jobs []T, workers int, fn func(T)) {
	if len(jobs) == 0 || workers <= 0 {
		return
	}

	jobChannel := make(chan T)
	var wg sync.WaitGroup

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for job := range jobChannel {
				runSafely("worker pool job", func() { fn(job) })
			}
		}()
	}

	for _, job := range jobs {
		jobChannel <- job
	}

	close(jobChannel)
	wg.Wait()
}
