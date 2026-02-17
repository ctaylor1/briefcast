package service

import (
	"fmt"
	"testing"
)

type testStringer struct {
	value string
}

func (t testStringer) String() string {
	return t.value
}

func TestParseFloat(t *testing.T) {
	cases := []struct {
		name     string
		input    interface{}
		expected float64
	}{
		{"float", 1.5, 1.5},
		{"int", 2, 2},
		{"int64", int64(3), 3},
		{"string", "4.25", 4.25},
		{"empty string", "", -1},
		{"invalid string", "x", -1},
		{"unsupported", true, -1},
	}

	for _, testCase := range cases {
		got := parseFloat(testCase.input)
		if got != testCase.expected {
			t.Fatalf("%s: expected %v, got %v", testCase.name, testCase.expected, got)
		}
	}
}

func TestStringValue(t *testing.T) {
	cases := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", "hello"},
		{"stringer", testStringer{value: "value"}, "value"},
		{"float64", 1.25, "1.25"},
		{"int", 7, "7"},
		{"int64", int64(8), "8"},
		{"map value", map[string]interface{}{"value": "v"}, "v"},
		{"map text", map[string]interface{}{"text": "t"}, "t"},
		{"map empty", map[string]interface{}{}, ""},
		{"unsupported", struct{}{}, ""},
	}

	for _, testCase := range cases {
		got := stringValue(testCase.input)
		if got != testCase.expected {
			t.Fatalf("%s: expected %q, got %q", testCase.name, testCase.expected, got)
		}
	}
}

func TestStringValueWithNestedStringerInMap(t *testing.T) {
	input := map[string]interface{}{
		"value": fmt.Stringer(testStringer{value: "nested"}),
	}
	if got := stringValue(input); got != "nested" {
		t.Fatalf("expected nested stringer result, got %q", got)
	}
}
