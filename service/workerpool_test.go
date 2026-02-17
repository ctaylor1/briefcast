package service

import (
	"sync"
	"testing"
)

func TestBoundedWorkerCount(t *testing.T) {
	cases := []struct {
		requested int
		fallback  int
		jobCount  int
		expected  int
	}{
		{requested: 0, fallback: 0, jobCount: 0, expected: 1},
		{requested: 0, fallback: 3, jobCount: 10, expected: 3},
		{requested: 5, fallback: 2, jobCount: 3, expected: 3},
		{requested: 2, fallback: 1, jobCount: 10, expected: 2},
	}

	for _, testCase := range cases {
		got := boundedWorkerCount(testCase.requested, testCase.fallback, testCase.jobCount)
		if got != testCase.expected {
			t.Fatalf("expected %d, got %d", testCase.expected, got)
		}
	}
}

func TestRunWorkerPoolExecutesAllJobs(t *testing.T) {
	jobs := []int{1, 2, 3, 4, 5}
	seen := map[int]bool{}
	var mu sync.Mutex

	runWorkerPool(jobs, 3, func(job int) {
		mu.Lock()
		seen[job] = true
		mu.Unlock()
	})

	if len(seen) != len(jobs) {
		t.Fatalf("expected %d processed jobs, got %d", len(jobs), len(seen))
	}
	for _, job := range jobs {
		if !seen[job] {
			t.Fatalf("job %d was not processed", job)
		}
	}
}

func TestRunWorkerPoolNoopConditions(t *testing.T) {
	calls := 0
	runWorkerPool([]int{}, 2, func(_ int) { calls++ })
	runWorkerPool([]int{1, 2}, 0, func(_ int) { calls++ })
	if calls != 0 {
		t.Fatalf("expected no calls, got %d", calls)
	}
}
