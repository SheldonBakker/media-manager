package config

import (
	"strings"
	"testing"
)

func TestAccountServiceVariable(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		problem bool
	}{
		{"not set leaves sign-up off", "", "", false},
		{"an https address is used", "https://api.example.com", "https://api.example.com", false},
		{"a trailing slash and spaces are dropped", "  https://api.example.com/ \n", "https://api.example.com", false},
		{"an address with a path keeps it", "https://example.com/accounts/", "https://example.com/accounts", false},
		{"plain http is allowed", "http://10.0.0.5:5180", "http://10.0.0.5:5180", false},
		{"a bare host name leaves sign-up off", "api.example.com", "", true},
		{"another scheme leaves sign-up off", "ftp://api.example.com", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MEDIARIUM_ACCOUNT_SERVICE_URL", tc.value)
			if got := Load().AccountServiceURL; got != tc.want {
				t.Fatalf("address = %q, want %q", got, tc.want)
			}
			problems := strings.Join(Problems(), "\n")
			if tc.problem != strings.Contains(problems, "MEDIARIUM_ACCOUNT_SERVICE_URL") {
				t.Fatalf("problems = %q, want one about the address: %v", problems, tc.problem)
			}
		})
	}
}
