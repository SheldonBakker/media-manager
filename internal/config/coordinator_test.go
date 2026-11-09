package config

import (
	"strings"
	"testing"
)

// A Coordinator address that cannot work leaves subscriptions off and says so
// in the log; Mediarium still starts.
func TestCoordinatorVariable(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		problem bool
	}{
		{"not set leaves subscriptions off", "", "", false},
		{"an http address is used", "http://coordinator:8080", "http://coordinator:8080", false},
		{"a trailing slash and spaces are dropped", "  https://coordinator.example.com/ \n", "https://coordinator.example.com", false},
		{"a bare host name leaves subscriptions off", "coordinator:8080", "", true},
		{"another scheme leaves subscriptions off", "ftp://coordinator.example.com", "", true},
		{"no host leaves subscriptions off", "http://", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MEDIARIUM_COORDINATOR_URL", tc.value)
			if got := Load().CoordinatorURL; got != tc.want {
				t.Fatalf("address = %q, want %q", got, tc.want)
			}
			problems := strings.Join(Problems(), "\n")
			if tc.problem != strings.Contains(problems, "MEDIARIUM_COORDINATOR_URL") {
				t.Fatalf("problems = %q, want one about the address: %v", problems, tc.problem)
			}
		})
	}
}
