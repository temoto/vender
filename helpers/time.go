package helpers

import "time"

func intDurationDefault(x int, scale time.Duration, def time.Duration) time.Duration {
	if x == 0 {
		return def
	}
	return time.Duration(x) * scale
}
func IntMillisecondDefault(x int, def time.Duration) time.Duration {
	return intDurationDefault(x, time.Millisecond, def)
}
func IntSecondDefault(x int, def time.Duration) time.Duration {
	return intDurationDefault(x, time.Second, def)
}
func IntSecondConfigDefault(x int, def int) time.Duration {
	if x == 0 {
		return time.Duration(def) * time.Second
	}
	return time.Duration(x) * time.Second
}
