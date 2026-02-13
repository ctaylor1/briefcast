package service

import "sync"

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
				fn(job)
			}
		}()
	}

	for _, job := range jobs {
		jobChannel <- job
	}

	close(jobChannel)
	wg.Wait()
}
