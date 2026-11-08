package config

import (
	"strings"
	"testing"
)

func TestProvisioningTokenVariable(t *testing.T) {
	long := strings.Repeat("k", provisioningTokenMin)
	tests := []struct {
		name    string
		value   string
		want    string
		problem bool
	}{
		{"not set leaves it off", "", "", false},
		{"a long enough token is used", long, long, false},
		{"the spaces around it are dropped", "  " + long + "\n", long, false},
		{"a short token leaves it off", long[1:], "", true},
		{"spaces don't make a token long enough", " " + long[1:] + " ", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MEDIARIUM_PROVISIONING_TOKEN", tc.value)
			t.Setenv("MEDIARIUM_PROVISIONING_TOKEN_FILE", "")
			if got := Load().ProvisioningToken; got != tc.want {
				t.Fatalf("token = %q, want %q", got, tc.want)
			}
			problems := strings.Join(Problems(), "\n")
			if tc.problem != strings.Contains(problems, "MEDIARIUM_PROVISIONING_TOKEN") {
				t.Fatalf("problems = %q, want one about the token: %v", problems, tc.problem)
			}
			if tc.value != "" && strings.Contains(problems, strings.TrimSpace(tc.value)) {
				t.Fatalf("the token itself is in the log: %q", problems)
			}
		})
	}
}

func TestProvisioningTokenFromAFile(t *testing.T) {
	long := strings.Repeat("k", provisioningTokenMin)
	t.Setenv("MEDIARIUM_PROVISIONING_TOKEN", "")
	t.Setenv("MEDIARIUM_PROVISIONING_TOKEN_FILE", writeSecret(t, long+"\n"))
	if got := Load().ProvisioningToken; got != long {
		t.Fatalf("token = %q, want the file's contents", got)
	}
}
