package format

import "os"

// Truncate returns s unchanged if it is n runes or shorter.
// Otherwise it returns the first n runes followed by "...".
func Truncate(s string, n int) string {
	if n <= 0 {
		return "..."
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// EnvOr returns the value of the environment variable key,
// or fallback if the variable is empty or unset.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
