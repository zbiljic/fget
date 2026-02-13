package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathWithin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	base := filepath.Join(root, "repo")
	child := filepath.Join(base, "subdir")
	sibling := filepath.Join(root, "repo2")
	parent := root

	if err := os.MkdirAll(child, os.ModePerm); err != nil {
		t.Fatalf("MkdirAll(child) error = %v", err)
	}
	if err := os.MkdirAll(sibling, os.ModePerm); err != nil {
		t.Fatalf("MkdirAll(sibling) error = %v", err)
	}

	tests := []struct {
		name   string
		base   string
		target string
		want   bool
	}{
		{
			name:   "same path",
			base:   base,
			target: base,
			want:   true,
		},
		{
			name:   "child path",
			base:   base,
			target: child,
			want:   true,
		},
		{
			name:   "sibling path",
			base:   base,
			target: sibling,
			want:   false,
		},
		{
			name:   "parent path",
			base:   base,
			target: parent,
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := isPathWithin(tt.base, tt.target)
			if err != nil {
				t.Fatalf("isPathWithin() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("isPathWithin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureCwdOutsideTargets(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "repo")
	cwdInside := filepath.Join(target, "inner")
	cwdOutside := filepath.Join(tmp, "outside")

	if err := os.MkdirAll(cwdInside, os.ModePerm); err != nil {
		t.Fatalf("MkdirAll(cwdInside) error = %v", err)
	}
	if err := os.MkdirAll(cwdOutside, os.ModePerm); err != nil {
		t.Fatalf("MkdirAll(cwdOutside) error = %v", err)
	}

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCwd)
	}()

	if err := os.Chdir(cwdInside); err != nil {
		t.Fatalf("Chdir(cwdInside) error = %v", err)
	}

	if err := ensureCwdOutsideTargets([]string{target}); err == nil {
		t.Fatal("ensureCwdOutsideTargets() expected error for cwd inside target")
	}

	if err := os.Chdir(cwdOutside); err != nil {
		t.Fatalf("Chdir(cwdOutside) error = %v", err)
	}

	if err := ensureCwdOutsideTargets([]string{target}); err != nil {
		t.Fatalf("ensureCwdOutsideTargets() unexpected error = %v", err)
	}
}
