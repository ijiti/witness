package format

import (
	"os"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 0, "..."},
		{"abc", -1, "..."},
		// Rune-safe: multibyte characters are not split.
		{"日本語テスト", 3, "日本語..."},
		{"café", 4, "café"},
		{"café", 3, "caf..."},
	}
	for _, tc := range tests {
		got := Truncate(tc.s, tc.n)
		if got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("FORMAT_TEST_SET", "value")
	os.Unsetenv("FORMAT_TEST_UNSET")
	t.Setenv("FORMAT_TEST_EMPTY", "")

	tests := []struct {
		key      string
		fallback string
		want     string
	}{
		{"FORMAT_TEST_SET", "default", "value"},
		{"FORMAT_TEST_UNSET", "default", "default"},
		{"FORMAT_TEST_EMPTY", "default", "default"},
	}
	for _, tc := range tests {
		got := EnvOr(tc.key, tc.fallback)
		if got != tc.want {
			t.Errorf("EnvOr(%q, %q) = %q, want %q", tc.key, tc.fallback, got, tc.want)
		}
	}
}
