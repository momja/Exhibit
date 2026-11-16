package api

import "testing"

func TestEnvBool(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{"", false}, {"true", true}, {"1", true}, {"yes", true}, {" ON ", true},
		{"false", false}, {"0", false}, {"no", false}, {"off", false}, {"invalid", false},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("TEST_BOOLEAN", test.value)
			if got := envBool("TEST_BOOLEAN"); got != test.want {
				t.Fatalf("envBool = %v, want %v", got, test.want)
			}
		})
	}
}
