package helpers

import "time"

func IntSecondDefault(x int, def time.Duration) time.Duration {
	if x == 0 {
		return def
	}
	return time.Duration(x) * time.Second
}
