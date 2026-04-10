package util

import "time"

func DeferString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func DeferTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}
