package service

import (
	"fmt"
	"strconv"
)

func parseFloat(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		if typed == "" {
			return -1
		}
		if parsed, err := strconv.ParseFloat(typed, 64); err == nil {
			return parsed
		}
	}
	return -1
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case map[string]interface{}:
		if s := stringValue(typed["value"]); s != "" {
			return s
		}
		if s := stringValue(typed["text"]); s != "" {
			return s
		}
		return ""
	default:
		return ""
	}
}
