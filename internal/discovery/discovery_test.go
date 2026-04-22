package discovery

import (
	"testing"
)

func TestValidatePathComponent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty string is invalid",
			input:   "",
			wantErr: true,
		},
		{
			name:    "forward slash is invalid",
			input:   "foo/bar",
			wantErr: true,
		},
		{
			name:    "backslash is invalid",
			input:   "foo\\bar",
			wantErr: true,
		},
		{
			name:    "double dot traversal is invalid",
			input:   "..",
			wantErr: true,
		},
		{
			name:    "double dot embedded is invalid",
			input:   "foo..bar",
			wantErr: true,
		},
		{
			name:    "simple session UUID is valid",
			input:   "abc123def456",
			wantErr: false,
		},
		{
			name:    "UUID with hyphens is valid",
			input:   "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "project dir name with hyphens is valid",
			input:   "-home-alice-myapp",
			wantErr: false,
		},
		{
			name:    "single dot is valid (not a traversal and filepath.Base matches)",
			input:   ".",
			wantErr: false,
		},
		{
			name:    "normal filename with underscores is valid",
			input:   "my_session_file",
			wantErr: false,
		},
		{
			name:    "path with leading slash is invalid",
			input:   "/etc/passwd",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePathComponent(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validatePathComponent(%q) = nil, want error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validatePathComponent(%q) = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestValidatePathComponentDots(t *testing.T) {
	// Separate focused tests for dot variants.
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"..", true},        // double dot — traversal
		{"foo..bar", true}, // embedded double dot — invalid
		{"foo.bar", false}, // single dot in name — valid
		{".hidden", false}, // leading dot (hidden file) — valid if filepath.Base matches
	}
	for _, tc := range tests {
		err := validatePathComponent(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("validatePathComponent(%q) = nil, want error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validatePathComponent(%q) = %v, want nil", tc.input, err)
		}
	}
}

func TestDecodeDirName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "typical project path",
			input: "-home-alice-myapp",
			want:  "/home/alice/myapp",
		},
		{
			name:  "simple home path",
			input: "-home-user",
			want:  "/home/user",
		},
		{
			name:  "root slash",
			input: "",
			want:  "/",
		},
		{
			name:  "deep path",
			input: "-home-alice-projects-myapp",
			want:  "/home/alice/projects/myapp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeDirName(tc.input)
			if got != tc.want {
				t.Errorf("decodeDirName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
