package alterra

// Response code constants
const (
	RCSuccess            = "00"
	RCPending            = "10"
	RCWrongNumber        = "20"
	RCProductIssue       = "21"
	RCDuplicateOrder     = "22"
	RCConnectionTimeout  = "23"
	RCProviderCutoff     = "24"
	RCKWHOverlimit       = "25"
	RCPaymentOverlimit   = "26"
	RCBillPaidOrNotFound = "50"
	RCInvalidInquiry     = "51"
	RCCanceledByOps      = "98"
	RCGeneralError       = "99"
)

// Status constants
const (
	StatusSuccess = "Success"
	StatusPending = "Pending"
	StatusFailed  = "Failed"
)

// successCodes are RC values that indicate success
var successCodes = map[string]bool{
	RCSuccess: true,
}

// pendingCodes are RC values that indicate pending
var pendingCodes = map[string]bool{
	RCPending: true,
}

// fatalCodes are RC values that indicate definite failure (no retry)
var fatalCodes = map[string]bool{
	RCWrongNumber:        true,
	RCProductIssue:       true,
	RCDuplicateOrder:     true,
	RCKWHOverlimit:       true,
	RCPaymentOverlimit:   true,
	RCBillPaidOrNotFound: true,
	RCInvalidInquiry:     true,
	RCCanceledByOps:      true,
}

// retryableCodes indicate temporary failures that can be retried
var retryableCodes = map[string]bool{
	RCConnectionTimeout: true,
	RCProviderCutoff:    true,
	RCGeneralError:      true,
}

// IsSuccess returns true if RC indicates success
func IsSuccess(rc string) bool {
	return successCodes[rc]
}

// IsPending returns true if RC indicates pending
func IsPending(rc string) bool {
	return pendingCodes[rc]
}

// IsFatal returns true if RC indicates definite failure
func IsFatal(rc string) bool {
	if rc == "" {
		return false
	}
	return !IsSuccess(rc) && !IsPending(rc)
}

// IsRetryable returns true if RC indicates temporary failure that can be retried
func IsRetryable(rc string) bool {
	return retryableCodes[rc]
}

// NeedsNewRefID returns true if a new reference ID is required for retry
func NeedsNewRefID(rc string) bool {
	return rc == RCDuplicateOrder
}

// GetRCDescription returns human-readable description
func GetRCDescription(rc string) string {
	descriptions := map[string]string{
		RCSuccess:            "Transaction successful",
		RCPending:            "Transaction is being processed",
		RCWrongNumber:        "Wrong Number/Blocked/Expired",
		RCProductIssue:       "Product Issue",
		RCDuplicateOrder:     "Duplicate Transactions",
		RCConnectionTimeout:  "Connection Timeout",
		RCProviderCutoff:     "Provider Cutoff",
		RCKWHOverlimit:       "KWH Overlimit",
		RCPaymentOverlimit:   "Payment Overlimit",
		RCBillPaidOrNotFound: "Bill Already Paid/Not Available",
		RCInvalidInquiry:     "Invalid Inquiry Amount/No Inquiry Found",
		RCCanceledByOps:      "Order Canceled by Ops",
		RCGeneralError:       "General Error",
	}
	if desc, ok := descriptions[rc]; ok {
		return desc
	}
	return "Unknown error"
}

// GetStatusFromRC converts RC to status string
func GetStatusFromRC(rc string) string {
	if IsSuccess(rc) {
		return StatusSuccess
	}
	if IsPending(rc) {
		return StatusPending
	}
	return StatusFailed
}
