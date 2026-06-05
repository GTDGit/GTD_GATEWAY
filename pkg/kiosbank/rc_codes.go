package kiosbank

// ResponsePhase describes which Kiosbank flow produced the RC.
type ResponsePhase string

const (
	ResponsePhaseInquiry        ResponsePhase = "inquiry"
	ResponsePhaseInitialPayment ResponsePhase = "initial_payment"
	ResponsePhaseAsync          ResponsePhase = "async"
)

// ResponseClass is the derived transaction state for a Kiosbank RC in a given phase.
type ResponseClass string

const (
	ResponseClassSuccess ResponseClass = "success"
	ResponseClassPending ResponseClass = "pending"
	ResponseClassFailed  ResponseClass = "failed"
)

// Response code constants
const (
	RCSuccess              = "00"
	RCUndefinedError       = "01"
	RCFormatError          = "02"
	RCBillerNoResponse     = "03"
	RCNoResponseFromBiller = "04"
	RCNoResponseFromHost   = "05"
	RCCannotProcess        = "10"
	RCInsufficientBalance  = "12"
	RCStorageIssue         = "14"
	RCUnknownProduct       = "15"
	RCTransactionFailed    = "17"
	RCNotRegistered        = "18"
	RCDataNotFound         = "19"
	RCCannotTransact       = "27"
	RCSessionExpired       = "34"
	RCAdminError           = "37"
	RCBillerLinkDown       = "38"
	RCTimeout              = "39"
	RCUnknownMessage       = "40"
	RCNotAuthorized        = "41"
	RCInvalidPrice         = "42"
	RCExpired              = "46"
	RCPayAtOffice          = "60"
	RCInquiryRequired      = "61"
	RCBillNotAvailable     = "62"
	RCAlreadyPaid          = "64"
	RCInvalidCustomer      = "65"
	RCProcessFailure       = "67"
	RCDailyLimitReached    = "68"
	RCNumberNotAllowed     = "69"
	RCInvalidAmount        = "70"
	RCProcessing           = "71"
	RCExceedsMaxPayment    = "72"
	RCRoutingError         = "73"
	RCAlreadySettled       = "74"
	RCMinInterval          = "75"
	RCExpiredNumber        = "78"
	RCExceedsMaxTunggakan  = "79"
	RCNoDataOrPaid         = "80"
	RCBlocked              = "83"
	RCCutOff               = "85"
	RCDuplicateRef         = "86"
)

// initialPaymentPendingCodes are RC values that indicate pending/processing
// for initial Payment / SinglePayment.
var initialPaymentPendingCodes = map[string]bool{
	RCUndefinedError:       true, // 01: Payment=PENDING per docs
	RCBillerNoResponse:     true, // 03
	RCNoResponseFromBiller: true, // 04
	RCNoResponseFromHost:   true, // 05
	RCStorageIssue:         true, // 14
	RCDataNotFound:         true, // 19: Payment=PENDING per docs
	RCBillerLinkDown:       true, // 38
	RCTimeout:              true, // 39
	RCProcessFailure:       true, // 67: Payment=PENDING per docs
	RCProcessing:           true, // 71
}

// sessionExpiredCodes indicate session needs refresh
var sessionExpiredCodes = map[string]bool{
	RCSessionExpired: true,
}

// ClassifyRC returns the live-doc transaction state for an RC in a specific phase.
func ClassifyRC(rc string, phase ResponsePhase) ResponseClass {
	switch phase {
	case ResponsePhaseInquiry:
		if rc == RCSuccess {
			return ResponseClassSuccess
		}
		return ResponseClassFailed
	case ResponsePhaseAsync:
		if rc == RCSuccess {
			return ResponseClassSuccess
		}
		if rc == RCTransactionFailed {
			return ResponseClassFailed
		}
		return ResponseClassPending
	default:
		if rc == RCSuccess {
			return ResponseClassSuccess
		}
		if initialPaymentPendingCodes[rc] {
			return ResponseClassPending
		}
		return ResponseClassFailed
	}
}

// StatusFromClass converts a response class into the service-level status string.
func StatusFromClass(class ResponseClass) string {
	switch class {
	case ResponseClassSuccess:
		return "Success"
	case ResponseClassPending:
		return "Pending"
	default:
		return "Failed"
	}
}

// IsSuccess returns true if RC indicates success
func IsSuccess(rc string) bool {
	return ClassifyRC(rc, ResponsePhaseInitialPayment) == ResponseClassSuccess
}

// IsPending returns true if RC indicates pending for the initial payment flow.
func IsPending(rc string) bool {
	return ClassifyRC(rc, ResponsePhaseInitialPayment) == ResponseClassPending
}

// IsFatal returns true if RC indicates failed for the initial payment flow.
func IsFatal(rc string) bool {
	return ClassifyRC(rc, ResponsePhaseInitialPayment) == ResponseClassFailed
}

// IsSessionExpired returns true if session needs to be refreshed
func IsSessionExpired(rc string) bool {
	return sessionExpiredCodes[rc]
}

// NeedsNewRefID returns true if a new reference ID is required
func NeedsNewRefID(rc string) bool {
	return rc == RCDuplicateRef
}

// GetRCDescription returns human-readable description
func GetRCDescription(rc string) string {
	descriptions := map[string]string{
		RCSuccess:              "Transaksi berhasil",
		RCUndefinedError:       "Error tidak terdefinisi",
		RCFormatError:          "Format message salah",
		RCBillerNoResponse:     "Tidak diresponse oleh biller",
		RCNoResponseFromBiller: "No response from biller",
		RCNoResponseFromHost:   "No response from host",
		RCCannotProcess:        "Transaksi tidak dapat dilakukan",
		RCInsufficientBalance:  "Saldo tidak mencukupi",
		RCStorageIssue:         "Kendala media penyimpanan",
		RCUnknownProduct:       "Produk tidak dikenal",
		RCTransactionFailed:    "Transaksi gagal",
		RCNotRegistered:        "Switching hulu belum registrasi",
		RCDataNotFound:         "Data tidak ditemukan",
		RCCannotTransact:       "Transaksi tidak dapat dilakukan",
		RCSessionExpired:       "Session expired",
		RCAdminError:           "Error setting admin bank",
		RCBillerLinkDown:       "Link ke biller terputus",
		RCTimeout:              "Timeout from biller",
		RCUnknownMessage:       "Jenis message tidak dikenal",
		RCNotAuthorized:        "Tidak diperbolehkan transaksi produk ini",
		RCInvalidPrice:         "Harga jual tidak valid",
		RCExpired:              "Transaksi sudah kadaluwarsa",
		RCPayAtOffice:          "Bayar di kantor PDAM",
		RCInquiryRequired:      "Harus inquiry terlebih dahulu",
		RCBillNotAvailable:     "Tagihan baru belum tersedia",
		RCAlreadyPaid:          "Tagihan sudah terbayar",
		RCInvalidCustomer:      "Nomor pelanggan salah",
		RCProcessFailure:       "Kegagalan saat proses transaksi",
		RCDailyLimitReached:    "Limit transaksi per hari tercapai",
		RCNumberNotAllowed:     "Nomor tidak diperbolehkan transaksi",
		RCInvalidAmount:        "Nominal pembayaran salah",
		RCProcessing:           "Transaksi sedang diproses",
		RCExceedsMaxPayment:    "Pembayaran melebihi batas maksimal",
		RCRoutingError:         "Kegagalan routing transaksi",
		RCAlreadySettled:       "Transaksi sudah lunas",
		RCMinInterval:          "Jeda minimal 10 menit untuk nominal sama",
		RCExpiredNumber:        "Nomor sudah kadaluwarsa",
		RCExceedsMaxTunggakan:  "Tunggakan melebihi maksimal",
		RCNoDataOrPaid:         "Data tidak tersedia atau sudah lunas",
		RCBlocked:              "Nomor sedang diblokir",
		RCCutOff:               "Sedang dalam masa cut off",
		RCDuplicateRef:         "Kode referensi sudah digunakan",
	}
	if desc, ok := descriptions[rc]; ok {
		return desc
	}
	return "Unknown error"
}
