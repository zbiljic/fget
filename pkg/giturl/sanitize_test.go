package giturl

import "testing"

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "https credentials", raw: "https://user:secret@example.com/acme/repo.git", want: "https://example.com/acme/repo.git"},
		{name: "ssh username", raw: "git@example.com:acme/repo.git", want: "ssh://git@example.com/acme/repo.git"},
		{name: "ssh password", raw: "ssh://git:secret@example.com/acme/repo.git", want: "ssh://git@example.com/acme/repo.git"},
		{name: "malformed credential URL", raw: "https://user:secret%zz@example.com/acme/repo.git", want: ""},
		{name: "empty", raw: "  ", want: ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Sanitize(test.raw); got != test.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
