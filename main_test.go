package main

import "testing"

func TestStateHelperProtocolCapability(t *testing.T) {
	if got := run([]string{"state-helper-version"}); got != 0 {
		t.Fatalf("run(state-helper-version) = %d, want 0", got)
	}
}
