package alterra

import "testing"

func TestIsFatalTreatsNonPendingNonSuccessAsFailed(t *testing.T) {
	t.Parallel()

	for _, rc := range []string{RCConnectionTimeout, RCProviderCutoff, RCGeneralError, RCWrongNumber} {
		if !IsFatal(rc) {
			t.Fatalf("IsFatal(%q) = false, want true", rc)
		}
	}

	if IsFatal(RCSuccess) {
		t.Fatalf("IsFatal(%q) = true, want false", RCSuccess)
	}
	if IsFatal(RCPending) {
		t.Fatalf("IsFatal(%q) = true, want false", RCPending)
	}
}
