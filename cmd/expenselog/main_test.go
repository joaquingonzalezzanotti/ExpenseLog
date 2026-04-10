package main

import "testing"

func TestShouldNoIndexPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/", want: false},
		{path: "/robots.txt", want: false},
		{path: "/sitemap.xml", want: false},
		{path: "/app", want: true},
		{path: "/app/settings", want: true},
		{path: "/api/auth/login", want: true},
		{path: "/style.css", want: false},
	}

	for _, tc := range tests {
		if got := shouldNoIndexPath(tc.path); got != tc.want {
			t.Fatalf("shouldNoIndexPath(%q) = %v want %v", tc.path, got, tc.want)
		}
	}
}
