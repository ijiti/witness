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

func TestGroupProjects(t *testing.T) {
	tests := []struct {
		name     string
		projects []Project
		// want maps a parent CWD ("" for ungrouped) to the slice of child CWDs
		want map[string][]string
	}{
		{
			name:     "empty input",
			projects: nil,
			want:     map[string][]string{},
		},
		{
			name: "single ungrouped project",
			projects: []Project{
				{ID: "a", CWD: "/home/alice/myapp"},
			},
			want: map[string][]string{
				"/home/alice/myapp": nil,
			},
		},
		{
			name: "path-prefix nesting (worktrees inside .git)",
			projects: []Project{
				{ID: "repo", CWD: "/home/alice/repo"},
				{ID: "wt1", CWD: "/home/alice/repo/.git/worktrees/feature-x"},
				{ID: "wt2", CWD: "/home/alice/repo/.git/worktrees/feature-y"},
			},
			want: map[string][]string{
				"/home/alice/repo": {"/home/alice/repo/.git/worktrees/feature-x", "/home/alice/repo/.git/worktrees/feature-y"},
			},
		},
		{
			name: "sibling worktree convention (~/worktrees/<repo>-task-*)",
			projects: []Project{
				{ID: "repo", CWD: "/home/alice/myrepo"},
				{ID: "wt1", CWD: "/home/alice/worktrees/myrepo-task-abc123"},
				{ID: "wt2", CWD: "/home/alice/worktrees/myrepo-task-def456"},
			},
			want: map[string][]string{
				"/home/alice/myrepo": {"/home/alice/worktrees/myrepo-task-abc123", "/home/alice/worktrees/myrepo-task-def456"},
			},
		},
		{
			name: "unrelated similarly-named projects DO NOT group (no worktree marker in path)",
			projects: []Project{
				{ID: "a", CWD: "/home/alice/mytool"},
				{ID: "b", CWD: "/home/alice/mytool-experimental"}, // sibling but no /worktrees/
			},
			want: map[string][]string{
				"/home/alice/mytool":              nil,
				"/home/alice/mytool-experimental": nil,
			},
		},
		{
			name: "worktree without matching parent stays top-level",
			projects: []Project{
				{ID: "wt", CWD: "/home/alice/worktrees/orphan-task-abc"},
			},
			want: map[string][]string{
				"/home/alice/worktrees/orphan-task-abc": nil,
			},
		},
		{
			name: "longer parent name wins over shorter",
			projects: []Project{
				{ID: "short", CWD: "/home/alice/my"},
				{ID: "long", CWD: "/home/alice/my-app"},
				{ID: "wt", CWD: "/home/alice/worktrees/my-app-task-abc"},
			},
			want: map[string][]string{
				"/home/alice/my":     nil,
				"/home/alice/my-app": {"/home/alice/worktrees/my-app-task-abc"},
			},
		},
		{
			name: "multiple unrelated repos",
			projects: []Project{
				{ID: "r1", CWD: "/home/alice/repo1"},
				{ID: "r2", CWD: "/home/alice/repo2"},
				{ID: "wt1", CWD: "/home/alice/worktrees/repo1-task-aaa"},
			},
			want: map[string][]string{
				"/home/alice/repo1": {"/home/alice/worktrees/repo1-task-aaa"},
				"/home/alice/repo2": nil,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupProjects(tc.projects)

			if len(got) != len(tc.want) {
				t.Errorf("got %d top-level groups, want %d", len(got), len(tc.want))
			}
			for _, g := range got {
				wantKids, ok := tc.want[g.CWD]
				if !ok {
					t.Errorf("unexpected top-level group %q", g.CWD)
					continue
				}
				if len(g.Children) != len(wantKids) {
					t.Errorf("group %q has %d children, want %d", g.CWD, len(g.Children), len(wantKids))
					continue
				}
				gotKids := make(map[string]bool, len(g.Children))
				for _, c := range g.Children {
					gotKids[c.CWD] = true
				}
				for _, w := range wantKids {
					if !gotKids[w] {
						t.Errorf("group %q missing expected child %q", g.CWD, w)
					}
				}
			}
		})
	}
}
