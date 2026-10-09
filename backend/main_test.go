package main

import "testing"

func TestProjectValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    project
		want bool
	}{
		{"valid", project{Title: "Example", Description: "Description", URL: "https://example.com"}, true},
		{"javascript URL", project{Title: "Example", Description: "Description", URL: "javascript:alert(1)"}, false},
		{"blank title", project{Title: " ", Description: "Description"}, false},
		{"relative URL", project{Title: "Example", Description: "Description", URL: "/project"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validProject(tc.p); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
func TestSessionDigest(t *testing.T) {
	if digest("secret") == "secret" || len(digest("secret")) != 64 {
		t.Fatal("session tokens must be hashed")
	}
}
