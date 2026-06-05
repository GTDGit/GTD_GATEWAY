package kiosbank

import "testing"

func TestClassifyRCByPhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rc    string
		phase ResponsePhase
		want  ResponseClass
	}{
		{name: "inquiry success", rc: RCSuccess, phase: ResponsePhaseInquiry, want: ResponseClassSuccess},
		{name: "inquiry non-zero failed", rc: RCSessionExpired, phase: ResponsePhaseInquiry, want: ResponseClassFailed},
		{name: "initial payment pending", rc: RCProcessing, phase: ResponsePhaseInitialPayment, want: ResponseClassPending},
		{name: "initial payment failed", rc: RCTransactionFailed, phase: ResponsePhaseInitialPayment, want: ResponseClassFailed},
		{name: "async success", rc: RCSuccess, phase: ResponsePhaseAsync, want: ResponseClassSuccess},
		{name: "async failed", rc: RCTransactionFailed, phase: ResponsePhaseAsync, want: ResponseClassFailed},
		{name: "async still pending", rc: RCProcessing, phase: ResponsePhaseAsync, want: ResponseClassPending},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyRC(tt.rc, tt.phase); got != tt.want {
				t.Fatalf("ClassifyRC(%q, %q) = %q, want %q", tt.rc, tt.phase, got, tt.want)
			}
		})
	}
}
