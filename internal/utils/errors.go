package utils

import "errors"

// Common application errors used across services.
var (
    ErrInvalidToken           = errors.New("INVALID_TOKEN")
    ErrInvalidClient          = errors.New("INVALID_CLIENT")
    ErrInvalidIP              = errors.New("INVALID_IP")
    ErrInvalidType            = errors.New("INVALID_TYPE")
    ErrInvalidSKU             = errors.New("INVALID_SKU")
    ErrDuplicateReferenceID   = errors.New("DUPLICATE_REFERENCE_ID")
    ErrNoAvailableSKU         = errors.New("NO_AVAILABLE_SKU")
    ErrTransactionNotFound    = errors.New("TRANSACTION_NOT_FOUND")
    ErrInvalidTransactionType = errors.New("INVALID_TRANSACTION_TYPE")
    ErrReferenceMismatch      = errors.New("REFERENCE_MISMATCH")
    ErrSkuMismatch            = errors.New("SKU_MISMATCH")
    ErrCustomerMismatch       = errors.New("CUSTOMER_MISMATCH")
    ErrInquiryExpired         = errors.New("INQUIRY_EXPIRED")
    ErrInquiryAlreadyPaid     = errors.New("INQUIRY_ALREADY_PAID")
    ErrInsufficientBalance    = errors.New("INSUFFICIENT_BALANCE")
)
