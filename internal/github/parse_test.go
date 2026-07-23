package github

import "testing"

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		raw         string
		wantOwner   string
		wantRepo    string
		wantErr     bool
	}{
		{"https://github.com/acme/api", "acme", "api", false},
		{"https://github.com/acme/api.git", "acme", "api", false},
		{"acme/api", "acme", "api", false},
		{"", "", "", true},
		{"not-a-repo", "", "", true},
	}
	for _, tc := range cases {
		owner, repo, err := ParseRepoURL(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRepoURL(%q) expected error", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRepoURL(%q) unexpected err: %v", tc.raw, err)
			continue
		}
		if owner != tc.wantOwner || repo != tc.wantRepo {
			t.Errorf("ParseRepoURL(%q)=(%s,%s) want (%s,%s)", tc.raw, owner, repo, tc.wantOwner, tc.wantRepo)
		}
	}
}

func TestParseReviewableLinesAndPatchPreferred(t *testing.T) {
	patch := "@@ -1,1 +1,2 @@\n package main\n+func x() {}\n"
	lines := ParseReviewableLines(patch)
	if _, ok := lines[2]; !ok {
		t.Fatalf("expected line 2 reviewable, got %v", lines)
	}
}
