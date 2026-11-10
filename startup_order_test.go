package main

import "testing"

func TestStartupClearsJobLocksBeforeTheStatsWarmup(t *testing.T) {
	var order []string
	runStartupJobs(
		func() { order = append(order, "unlock") },
		func() { order = append(order, "warmup") },
	)
	if len(order) != 2 || order[0] != "unlock" || order[1] != "warmup" {
		t.Fatalf("startup order %v, want [unlock warmup]", order)
	}
}
