package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// Alterra RC constants (inlined from pkg/alterra).
const (
	alterraRCWrongNumber        = "20"
	alterraRCProductIssue       = "21"
	alterraRCDuplicateOrder     = "22"
	alterraRCConnectionTimeout  = "23"
	alterraRCProviderCutoff     = "24"
	alterraRCKWHOverlimit       = "25"
	alterraRCPaymentOverlimit   = "26"
	alterraRCBillPaidOrNotFound = "50"
	alterraRCInvalidInquiry     = "51"
	alterraRCCanceledByOps      = "98"
	alterraRCGeneralError       = "99"
)

// Kiosbank RC constants (inlined from pkg/kiosbank).
const (
	kiosbankRCNoResponseFromBiller = "04"
	kiosbankRCTimeout              = "39"
	kiosbankRCNoResponseFromHost   = "05"
	kiosbankRCStorageIssue         = "14"
	kiosbankRCNotRegistered        = "18"
	kiosbankRCBillerLinkDown       = "38"
	kiosbankRCCutOff               = "85"
	kiosbankRCUnknownProduct       = "15"
	kiosbankRCDataNotFound         = "19"
	kiosbankRCPayAtOffice          = "60"
	kiosbankRCBillNotAvailable     = "62"
	kiosbankRCNoDataOrPaid         = "80"
	kiosbankRCAlreadyPaid          = "64"
	kiosbankRCAlreadySettled       = "74"
	kiosbankRCInvalidCustomer      = "65"
	kiosbankRCExpiredNumber        = "78"
	kiosbankRCDailyLimitReached    = "68"
	kiosbankRCNumberNotAllowed     = "69"
	kiosbankRCBlocked              = "83"
	kiosbankRCInvalidAmount        = "70"
	kiosbankRCExceedsMaxPayment    = "72"
	kiosbankRCExceedsMaxTunggakan  = "79"
	kiosbankRCInquiryRequired      = "61"
	kiosbankRCExpired              = "46"
	kiosbankRCMinInterval          = "75"
	kiosbankRCFormatError          = "02"
	kiosbankRCAdminError           = "37"
	kiosbankRCUnknownMessage       = "40"
	kiosbankRCInvalidPrice         = "42"
	kiosbankRCInsufficientBalance  = "12"
	kiosbankRCTransactionFailed    = "17"
	kiosbankRCNotAuthorized        = "41"
	kiosbankRCDuplicateRef         = "86"
)

type ProviderFailurePhase string

const (
	ProviderFailurePhaseInquiry        ProviderFailurePhase = "inquiry"
	ProviderFailurePhaseInitialPayment ProviderFailurePhase = "initial_payment"
	ProviderFailurePhaseAsync          ProviderFailurePhase = "async"
)

const (
	ProviderFailureDuplicateTransaction        = "DUPLICATE_TRANSACTION"
	ProviderFailureAlreadyPaid                 = "ALREADY_PAID"
	ProviderFailureInquiryRequired             = "INQUIRY_REQUIRED"
	ProviderFailureRequestExpired              = "REQUEST_EXPIRED"
	ProviderFailureCooldownActive              = "COOLDOWN_ACTIVE"
	ProviderFailureOrderCanceled               = "ORDER_CANCELED"
	ProviderFailureInvalidCustomer             = "INVALID_CUSTOMER"
	ProviderFailureCustomerRestricted          = "CUSTOMER_RESTRICTED"
	ProviderFailureInvalidAmount               = "INVALID_AMOUNT"
	ProviderFailureInquiryNotFound             = "INQUIRY_NOT_FOUND"
	ProviderFailureBillUnavailable             = "BILL_UNAVAILABLE"
	ProviderFailureLimitExceeded               = "LIMIT_EXCEEDED"
	ProviderFailureProductUnavailable          = "PRODUCT_UNAVAILABLE"
	ProviderFailureProviderBalanceInsufficient = "PROVIDER_BALANCE_INSUFFICIENT"
	ProviderFailureProviderUnavailable         = "PROVIDER_UNAVAILABLE"
	ProviderFailureNoProviderAvailable         = "NO_PROVIDER_AVAILABLE"
	ProviderFailureProviderTimeout             = "PROVIDER_TIMEOUT"
	ProviderFailureUpstreamRequestInvalid      = "UPSTREAM_REQUEST_INVALID"
	ProviderFailureUpstreamAuthError           = "UPSTREAM_AUTH_ERROR"
	ProviderFailureGeneralProviderError        = "GENERAL_PROVIDER_ERROR"
)

type CanonicalProviderFailure struct {
	Code       string
	HTTPStatus int
	Message    string
}

var canonicalProviderFailures = map[string]CanonicalProviderFailure{
	ProviderFailureDuplicateTransaction:        {Code: ProviderFailureDuplicateTransaction, HTTPStatus: http.StatusConflict, Message: "Duplicate transaction"},
	ProviderFailureAlreadyPaid:                 {Code: ProviderFailureAlreadyPaid, HTTPStatus: http.StatusConflict, Message: "Bill or transaction has already been paid"},
	ProviderFailureInquiryRequired:             {Code: ProviderFailureInquiryRequired, HTTPStatus: http.StatusConflict, Message: "Inquiry must be completed before payment"},
	ProviderFailureRequestExpired:              {Code: ProviderFailureRequestExpired, HTTPStatus: http.StatusConflict, Message: "Transaction request has expired"},
	ProviderFailureCooldownActive:              {Code: ProviderFailureCooldownActive, HTTPStatus: http.StatusConflict, Message: "Similar transaction is temporarily restricted"},
	ProviderFailureOrderCanceled:               {Code: ProviderFailureOrderCanceled, HTTPStatus: http.StatusConflict, Message: "Transaction was canceled by upstream operations"},
	ProviderFailureInvalidCustomer:             {Code: ProviderFailureInvalidCustomer, HTTPStatus: http.StatusUnprocessableEntity, Message: "Customer number or account is invalid"},
	ProviderFailureCustomerRestricted:          {Code: ProviderFailureCustomerRestricted, HTTPStatus: http.StatusUnprocessableEntity, Message: "Customer is not allowed to transact"},
	ProviderFailureInvalidAmount:               {Code: ProviderFailureInvalidAmount, HTTPStatus: http.StatusUnprocessableEntity, Message: "Transaction amount is invalid"},
	ProviderFailureInquiryNotFound:             {Code: ProviderFailureInquiryNotFound, HTTPStatus: http.StatusUnprocessableEntity, Message: "Inquiry not found or amount has changed"},
	ProviderFailureBillUnavailable:             {Code: ProviderFailureBillUnavailable, HTTPStatus: http.StatusUnprocessableEntity, Message: "Bill is not available"},
	ProviderFailureLimitExceeded:               {Code: ProviderFailureLimitExceeded, HTTPStatus: http.StatusUnprocessableEntity, Message: "Transaction exceeds the allowed limit"},
	ProviderFailureProductUnavailable:          {Code: ProviderFailureProductUnavailable, HTTPStatus: http.StatusServiceUnavailable, Message: "Product is temporarily unavailable"},
	ProviderFailureProviderBalanceInsufficient: {Code: ProviderFailureProviderBalanceInsufficient, HTTPStatus: http.StatusServiceUnavailable, Message: "Transaction cannot be processed at the moment"},
	ProviderFailureProviderUnavailable:         {Code: ProviderFailureProviderUnavailable, HTTPStatus: http.StatusServiceUnavailable, Message: "Provider service is temporarily unavailable"},
	ProviderFailureNoProviderAvailable:         {Code: ProviderFailureNoProviderAvailable, HTTPStatus: http.StatusServiceUnavailable, Message: "No provider could complete the transaction"},
	ProviderFailureProviderTimeout:             {Code: ProviderFailureProviderTimeout, HTTPStatus: http.StatusGatewayTimeout, Message: "Provider did not respond in time"},
	ProviderFailureUpstreamRequestInvalid:      {Code: ProviderFailureUpstreamRequestInvalid, HTTPStatus: http.StatusBadGateway, Message: "Upstream request could not be processed"},
	ProviderFailureUpstreamAuthError:           {Code: ProviderFailureUpstreamAuthError, HTTPStatus: http.StatusBadGateway, Message: "Upstream authentication failed"},
	ProviderFailureGeneralProviderError:        {Code: ProviderFailureGeneralProviderError, HTTPStatus: http.StatusBadGateway, Message: "Transaction could not be completed"},
}

var providerFailurePrecedence = map[string]int{
	ProviderFailureInvalidCustomer:             10,
	ProviderFailureCustomerRestricted:          11,
	ProviderFailureInvalidAmount:               12,
	ProviderFailureInquiryNotFound:             13,
	ProviderFailureBillUnavailable:             14,
	ProviderFailureLimitExceeded:               15,
	ProviderFailureAlreadyPaid:                 16,
	ProviderFailureInquiryRequired:             17,
	ProviderFailureDuplicateTransaction:        18,
	ProviderFailureRequestExpired:              19,
	ProviderFailureCooldownActive:              20,
	ProviderFailureProductUnavailable:          30,
	ProviderFailureProviderBalanceInsufficient: 40,
	ProviderFailureUpstreamRequestInvalid:      41,
	ProviderFailureUpstreamAuthError:           42,
	ProviderFailureProviderUnavailable:         43,
	ProviderFailureProviderTimeout:             44,
	ProviderFailureGeneralProviderError:        45,
	ProviderFailureNoProviderAvailable:         99,
}

func GetCanonicalProviderFailure(code string) CanonicalProviderFailure {
	if failure, ok := canonicalProviderFailures[code]; ok {
		return failure
	}
	return canonicalProviderFailures[ProviderFailureGeneralProviderError]
}

func ProviderFailurePhaseForTransactionType(trxType models.TransactionType) ProviderFailurePhase {
	if trxType == models.TrxTypeInquiry {
		return ProviderFailurePhaseInquiry
	}
	return ProviderFailurePhaseInitialPayment
}

func CanonicalFailureForResponse(providerCode string, phase ProviderFailurePhase, resp *ProviderResponse) CanonicalProviderFailure {
	if resp == nil {
		return GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable)
	}
	if resp.PublicCode != "" {
		return GetCanonicalProviderFailure(resp.PublicCode)
	}

	var failure CanonicalProviderFailure
	switch providerCode {
	case string(models.ProviderAlterra):
		failure = canonicalAlterraFailure(resp)
	case string(models.ProviderKiosbank):
		failure = canonicalKiosbankFailure(phase, resp)
	default:
		failure = canonicalGenericFailure(resp)
	}

	resp.PublicCode = failure.Code
	resp.PublicMessage = failure.Message
	resp.PublicHTTPCode = failure.HTTPStatus
	return failure
}

func ApplyCanonicalFailureToTransaction(trx *models.Transaction, providerCode string, phase ProviderFailurePhase, resp *ProviderResponse) CanonicalProviderFailure {
	failure := CanonicalFailureForResponse(providerCode, phase, resp)
	code := failure.Code
	message := failure.Message
	trx.FailedCode = &code
	trx.FailedReason = &message
	if resp != nil {
		if desc := SanitizePublicProviderDescription(resp.Description); len(desc) > 0 {
			trx.Description = models.NullableRawMessage(desc)
		}
	}
	return failure
}

func SanitizePublicProviderDescription(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		if sanitized := sanitizePublicProviderString(string(raw)); sanitized != "" && sanitized != string(raw) {
			out, _ := json.Marshal(map[string]any{"detail": sanitized})
			return out
		}
		cp := make([]byte, len(raw))
		copy(cp, raw)
		return cp
	}

	sanitized := sanitizeProviderDescriptionValue(payload)
	out, err := json.Marshal(sanitized)
	if err != nil {
		return nil
	}
	if string(out) == "null" || string(out) == "{}" {
		return nil
	}
	return out
}

func BuildFinalFailureResponseFromAttempts(attempts []ProviderAttempt, phase ProviderFailurePhase) *ProviderResponse {
	if len(attempts) == 0 {
		resp := &ProviderResponse{
			Status: string(models.StatusFailed),
		}
		failure := GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable)
		resp.PublicCode = failure.Code
		resp.PublicMessage = failure.Message
		resp.PublicHTTPCode = failure.HTTPStatus
		return resp
	}

	best := GetCanonicalProviderFailure(ProviderFailureNoProviderAvailable)
	bestRank := providerFailurePrecedence[best.Code]
	var chosen *ProviderResponse

	for i := range attempts {
		attempt := attempts[i]
		providerCode := ""
		if attempt.Provider != nil {
			providerCode = string(attempt.Provider.ProviderCode)
		}

		var resp *ProviderResponse
		if attempt.Response != nil {
			resp = attempt.Response
		}

		if resp == nil && attempt.Error != "" {
			resp = providerResponseFromError(providerCode, phase, errString(attempt.Error))
		}
		if resp == nil {
			continue
		}

		failure := CanonicalFailureForResponse(providerCode, phase, resp)
		rank := providerFailurePrecedence[failure.Code]
		if chosen == nil || rank < bestRank {
			best = failure
			bestRank = rank
			chosen = resp
		}
	}

	if chosen == nil {
		chosen = &ProviderResponse{Status: string(models.StatusFailed)}
	}
	chosen.PublicCode = best.Code
	chosen.PublicMessage = best.Message
	chosen.PublicHTTPCode = best.HTTPStatus
	if chosen.Message == "" {
		chosen.Message = best.Message
	}
	if len(chosen.Description) > 0 {
		chosen.Description = SanitizePublicProviderDescription(chosen.Description)
	}
	return chosen
}

func canonicalAlterraFailure(resp *ProviderResponse) CanonicalProviderFailure {
	httpStatus := resp.HTTPStatus
	rc := strings.TrimSpace(resp.RC)
	lowerRC := strings.ToLower(rc)
	lowerMessage := strings.ToLower(strings.TrimSpace(resp.Message))
	rawCode, rawMessage := extractRawProviderError(resp.RawResponse)
	lowerRawCode := strings.ToLower(rawCode)
	lowerRawMessage := strings.ToLower(rawMessage)

	switch {
	case lowerRC == alterraRCWrongNumber:
		return GetCanonicalProviderFailure(ProviderFailureInvalidCustomer)
	case lowerRC == alterraRCProductIssue:
		return GetCanonicalProviderFailure(ProviderFailureProductUnavailable)
	case lowerRC == alterraRCDuplicateOrder:
		return GetCanonicalProviderFailure(ProviderFailureDuplicateTransaction)
	case lowerRC == alterraRCConnectionTimeout:
		return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
	case lowerRC == alterraRCProviderCutoff:
		return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
	case lowerRC == alterraRCKWHOverlimit || lowerRC == alterraRCPaymentOverlimit:
		return GetCanonicalProviderFailure(ProviderFailureLimitExceeded)
	case lowerRC == alterraRCBillPaidOrNotFound:
		return GetCanonicalProviderFailure(ProviderFailureAlreadyPaid)
	case lowerRC == alterraRCInvalidInquiry:
		return GetCanonicalProviderFailure(ProviderFailureInquiryNotFound)
	case lowerRC == alterraRCCanceledByOps:
		return GetCanonicalProviderFailure(ProviderFailureOrderCanceled)
	case lowerRC == alterraRCGeneralError:
		return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
	case strings.Contains(lowerRC, "product_closed"), strings.Contains(lowerRawCode, "product_closed"), strings.Contains(lowerMessage, "product closed"), strings.Contains(lowerRawMessage, "product closed"), httpStatus == 450:
		return GetCanonicalProviderFailure(ProviderFailureProductUnavailable)
	case strings.Contains(lowerRC, "insufficient_deposit"), strings.Contains(lowerRawCode, "insufficient_deposit"), strings.Contains(lowerMessage, "insufficient deposit"), strings.Contains(lowerRawMessage, "insufficient deposit"):
		return GetCanonicalProviderFailure(ProviderFailureProviderBalanceInsufficient)
	case strings.Contains(lowerMessage, "duplicate order id"), strings.Contains(lowerRawMessage, "duplicate order id"), httpStatus == http.StatusUnprocessableEntity:
		return GetCanonicalProviderFailure(ProviderFailureDuplicateTransaction)
	case httpStatus == http.StatusBadRequest || httpStatus == http.StatusNotAcceptable:
		return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
	case httpStatus == http.StatusUnauthorized:
		return GetCanonicalProviderFailure(ProviderFailureUpstreamAuthError)
	case httpStatus == http.StatusForbidden && (strings.Contains(lowerMessage, "invalid product") || strings.Contains(lowerRawMessage, "invalid product") || strings.Contains(lowerMessage, "product unavailable") || strings.Contains(lowerRawMessage, "product unavailable")):
		return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
	case httpStatus == http.StatusForbidden:
		return GetCanonicalProviderFailure(ProviderFailureUpstreamAuthError)
	case httpStatus == http.StatusGatewayTimeout:
		return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
	}

	if looksLikeTransportTimeout(resp) {
		return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
	}
	if looksLikeTransportFailure(resp) {
		return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
	}

	if strings.HasPrefix(rc, "4") {
		return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
	}

	return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
}

func canonicalKiosbankFailure(phase ProviderFailurePhase, resp *ProviderResponse) CanonicalProviderFailure {
	rc := strings.TrimSpace(resp.RC)

	if looksLikeTransportTimeout(resp) {
		if phase == ProviderFailurePhaseInitialPayment || phase == ProviderFailurePhaseAsync {
			return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
		}
		return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
	}
	if looksLikeTransportFailure(resp) {
		return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
	}

	if resp.PublicCode != "" {
		return GetCanonicalProviderFailure(resp.PublicCode)
	}

	switch phase {
	case ProviderFailurePhaseInquiry:
		switch rc {
		case kiosbankRCNoResponseFromBiller, kiosbankRCTimeout:
			return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
		case kiosbankRCNoResponseFromHost, kiosbankRCStorageIssue, kiosbankRCNotRegistered, kiosbankRCBillerLinkDown, kiosbankRCCutOff:
			return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
		case kiosbankRCUnknownProduct:
			return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
		case kiosbankRCDataNotFound:
			return GetCanonicalProviderFailure(ProviderFailureInquiryNotFound)
		case kiosbankRCPayAtOffice, kiosbankRCBillNotAvailable, kiosbankRCNoDataOrPaid:
			return GetCanonicalProviderFailure(ProviderFailureBillUnavailable)
		case kiosbankRCAlreadyPaid, kiosbankRCAlreadySettled:
			return GetCanonicalProviderFailure(ProviderFailureAlreadyPaid)
		case kiosbankRCInvalidCustomer, kiosbankRCExpiredNumber:
			return GetCanonicalProviderFailure(ProviderFailureInvalidCustomer)
		case kiosbankRCDailyLimitReached, kiosbankRCNumberNotAllowed, kiosbankRCBlocked:
			return GetCanonicalProviderFailure(ProviderFailureCustomerRestricted)
		case kiosbankRCInvalidAmount:
			return GetCanonicalProviderFailure(ProviderFailureInvalidAmount)
		case kiosbankRCExceedsMaxPayment, kiosbankRCExceedsMaxTunggakan:
			return GetCanonicalProviderFailure(ProviderFailureLimitExceeded)
		case kiosbankRCInquiryRequired:
			return GetCanonicalProviderFailure(ProviderFailureInquiryRequired)
		case kiosbankRCExpired:
			return GetCanonicalProviderFailure(ProviderFailureRequestExpired)
		case kiosbankRCMinInterval:
			return GetCanonicalProviderFailure(ProviderFailureCooldownActive)
		case kiosbankRCFormatError, kiosbankRCAdminError, kiosbankRCUnknownMessage, kiosbankRCInvalidPrice:
			return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
		case kiosbankRCInsufficientBalance:
			return GetCanonicalProviderFailure(ProviderFailureProviderBalanceInsufficient)
		default:
			return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
		}
	case ProviderFailurePhaseAsync:
		if rc == kiosbankRCTransactionFailed {
			return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
		}
		return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
	default:
		switch rc {
		case kiosbankRCTransactionFailed:
			return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
		case kiosbankRCInsufficientBalance:
			return GetCanonicalProviderFailure(ProviderFailureProviderBalanceInsufficient)
		case kiosbankRCUnknownProduct, kiosbankRCInvalidPrice, kiosbankRCFormatError, kiosbankRCAdminError, kiosbankRCUnknownMessage:
			return GetCanonicalProviderFailure(ProviderFailureUpstreamRequestInvalid)
		case kiosbankRCNotRegistered, kiosbankRCCutOff:
			return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
		case kiosbankRCNotAuthorized:
			return GetCanonicalProviderFailure(ProviderFailureProductUnavailable)
		case kiosbankRCExpired:
			return GetCanonicalProviderFailure(ProviderFailureRequestExpired)
		case kiosbankRCPayAtOffice, kiosbankRCBillNotAvailable, kiosbankRCNoDataOrPaid:
			return GetCanonicalProviderFailure(ProviderFailureBillUnavailable)
		case kiosbankRCInquiryRequired:
			return GetCanonicalProviderFailure(ProviderFailureInquiryRequired)
		case kiosbankRCAlreadyPaid, kiosbankRCAlreadySettled:
			return GetCanonicalProviderFailure(ProviderFailureAlreadyPaid)
		case kiosbankRCInvalidCustomer, kiosbankRCExpiredNumber:
			return GetCanonicalProviderFailure(ProviderFailureInvalidCustomer)
		case kiosbankRCDailyLimitReached, kiosbankRCNumberNotAllowed, kiosbankRCBlocked:
			return GetCanonicalProviderFailure(ProviderFailureCustomerRestricted)
		case kiosbankRCInvalidAmount:
			return GetCanonicalProviderFailure(ProviderFailureInvalidAmount)
		case kiosbankRCExceedsMaxPayment, kiosbankRCExceedsMaxTunggakan:
			return GetCanonicalProviderFailure(ProviderFailureLimitExceeded)
		case kiosbankRCMinInterval:
			return GetCanonicalProviderFailure(ProviderFailureCooldownActive)
		case kiosbankRCDuplicateRef:
			return GetCanonicalProviderFailure(ProviderFailureDuplicateTransaction)
		default:
			return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
		}
	}
}

func canonicalGenericFailure(resp *ProviderResponse) CanonicalProviderFailure {
	if looksLikeTransportTimeout(resp) {
		return GetCanonicalProviderFailure(ProviderFailureProviderTimeout)
	}
	if looksLikeTransportFailure(resp) {
		return GetCanonicalProviderFailure(ProviderFailureProviderUnavailable)
	}
	return GetCanonicalProviderFailure(ProviderFailureGeneralProviderError)
}

func sanitizeProviderDescriptionValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			switch strings.ToLower(key) {
			case "transport_error", "url", "host", "endpoint":
				continue
			default:
				if sanitized := sanitizeProviderDescriptionValue(val); sanitized != nil {
					out[key] = sanitized
				}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			if sanitized := sanitizeProviderDescriptionValue(item); sanitized != nil {
				out = append(out, sanitized)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		return sanitizePublicProviderString(typed)
	default:
		return value
	}
}

func sanitizePublicProviderString(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return trimmed
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "http://") ||
		strings.Contains(lower, "https://") ||
		strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "tls:") ||
		strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "handshake timeout") ||
		strings.Contains(lower, "kiosbank") ||
		strings.Contains(lower, "alterra") {
		return "Provider transport error"
	}
	return trimmed
}

func extractRawProviderError(raw json.RawMessage) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	if errMap, ok := payload["error"].(map[string]any); ok {
		code, _ := errMap["code"].(string)
		message, _ := errMap["message"].(string)
		return code, message
	}
	if transportErr, ok := payload["transport_error"].(string); ok && strings.TrimSpace(transportErr) != "" {
		return "", transportErr
	}
	code, _ := payload["code"].(string)
	message, _ := payload["message"].(string)
	return code, message
}

func looksLikeTransportTimeout(resp *ProviderResponse) bool {
	if resp == nil {
		return false
	}
	rawCode, rawMessage := extractRawProviderError(resp.RawResponse)
	combined := strings.ToLower(strings.Join([]string{resp.RC, resp.Message, rawCode, rawMessage}, " "))
	return looksLikeTimeoutMessage(combined)
}

func looksLikeTransportFailure(resp *ProviderResponse) bool {
	if resp == nil {
		return false
	}
	rawCode, rawMessage := extractRawProviderError(resp.RawResponse)
	combined := strings.ToLower(strings.Join([]string{resp.RC, resp.Message, rawCode, rawMessage}, " "))
	return strings.Contains(combined, "transport error") ||
		strings.Contains(combined, "tls:") ||
		strings.Contains(combined, "dial tcp") ||
		strings.Contains(combined, "no response")
}

func looksLikeTimeoutMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded")
}

type stringError string

func (e stringError) Error() string {
	return string(e)
}

func errString(message string) error {
	return stringError(message)
}
