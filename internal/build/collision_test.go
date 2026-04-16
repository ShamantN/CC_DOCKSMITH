package build

import (
	"testing"
)

func TestSortedEnvCollisions(t *testing.T) {
	// Case 1: One variable with a semicolon in the value
	env1 := []string{"A=1;B=2"}
	// Case 2: Two separate variables
	env2 := []string{"A=1", "B=2"}

	res1 := sortedEnv(env1)
	res2 := sortedEnv(env2)

	if res1 == res2 {
		t.Errorf("COLLISION DETECTED: ENV 'A=1;B=2' and ENV 'A=1', 'B=2' produced the same serialized string: %q", res1)
	}
}

func TestSortedEnvDeterminism(t *testing.T) {
	env := []string{"Z=9", "A=1", "M=5"}
	res1 := sortedEnv(env)
	res2 := sortedEnv([]string{"A=1", "M=5", "Z=9"})

	if res1 != res2 {
		t.Errorf("Determinism failed: sortedEnv should produce identical results regardless of input order. Got %q and %q", res1, res2)
	}
}
