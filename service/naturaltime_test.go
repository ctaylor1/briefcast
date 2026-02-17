package service

import (
	"testing"
	"time"
)

func TestNatualTimeFutureCases(t *testing.T) {
	base := time.Date(2026, time.February, 17, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		value    time.Time
		expected string
	}{
		{base.Add(30 * time.Second), "in a few seconds"},
		{base.Add(3 * time.Minute), "in a few minutes"},
		{base.Add(15 * time.Minute), "in 15 minutes"},
		{base.Add(26 * time.Hour), "tomorrow"},
		{base.Add(50 * time.Hour), "day after tomorrow"},
		{base.Add(70 * 24 * time.Hour), "in 2 months"},
		{base.Add(400 * 24 * time.Hour), "next year"},
	}

	for _, testCase := range cases {
		got := NatualTime(base, testCase.value)
		if got != testCase.expected {
			t.Fatalf("expected %q, got %q for value %s", testCase.expected, got, testCase.value)
		}
	}
}

func TestNatualTimePastCases(t *testing.T) {
	base := time.Date(2026, time.February, 17, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		value    time.Time
		expected string
	}{
		{base.Add(-30 * time.Second), "a few seconds ago"},
		{base.Add(-3 * time.Minute), "a few minutes ago"},
		{base.Add(-30 * time.Minute), "30 minutes ago"},
		{time.Date(2026, time.February, 17, 8, 0, 0, 0, time.UTC), "4 hours ago"},
		{time.Date(2026, time.February, 16, 18, 0, 0, 0, time.UTC), "yesterday"},
		{time.Date(2026, time.February, 15, 18, 0, 0, 0, time.UTC), "day before yesterday"},
		{base.Add(-40 * 24 * time.Hour), "last month"},
		{base.Add(-800 * 24 * time.Hour), "2 years ago"},
	}

	for _, testCase := range cases {
		got := NatualTime(base, testCase.value)
		if got != testCase.expected {
			t.Fatalf("expected %q, got %q for value %s", testCase.expected, got, testCase.value)
		}
	}
}
