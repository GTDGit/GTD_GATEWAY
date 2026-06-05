package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/GTDGit/gtd_gateway/pkg/bnc"
	"github.com/GTDGit/gtd_gateway/pkg/pakailink"
)

// pakailinkTransferAdapter bridges pakailink.Client to the snapTransferClient
// interface so TransferService can use it as a drop-in disbursement provider.
//
// Note: PakaiLink does not differentiate intrabank vs interbank — all transfers
// go through Service 43 using the destination bank code. We therefore route
// IntrabankTransfer / InterbankTransfer / InternalAccountInquiry /
// ExternalAccountInquiry to the same underlying calls.
type pakailinkTransferAdapter struct {
	client      *pakailink.Client
	callbackURL string
}

// NewPakailinkTransferAdapter wraps a pakailink.Client to satisfy the
// disbursement transfer interface used by TransferService.
func NewPakailinkTransferAdapter(client *pakailink.Client, callbackURL string) *pakailinkTransferAdapter {
	return &pakailinkTransferAdapter{client: client, callbackURL: strings.TrimSpace(callbackURL)}
}

func (a *pakailinkTransferAdapter) ExternalAccountInquiry(ctx context.Context, bankCode, accountNo string) (*bnc.AccountInquiryResponse, error) {
	resp, err := a.client.BankAccountInquiry(ctx, pakailink.BankAccountInquiryRequest{
		PartnerReferenceNo:   newTransferPublicID("PLI"),
		BeneficiaryAccountNo: accountNo,
		BeneficiaryBankCode:  bankCode,
	})
	if err != nil {
		return nil, pakailinkAsBNCError(err)
	}
	return &bnc.AccountInquiryResponse{
		ResponseCode:           resp.ResponseCode,
		ResponseMessage:        resp.ResponseMessage,
		ReferenceNo:            resp.SessionID,
		BeneficiaryAccountName: resp.BeneficiaryAccountName,
		BeneficiaryAccountNo:   resp.BeneficiaryAccountNumber,
		BeneficiaryBankCode:    bankCode,
		BeneficiaryBankName:    resp.BeneficiaryBankName,
		Currency:               "IDR",
		RawResponse:            resp.RawResponse,
	}, nil
}

func (a *pakailinkTransferAdapter) InternalAccountInquiry(ctx context.Context, accountNo string) (*bnc.AccountInquiryResponse, error) {
	// PakaiLink has no intrabank concept; the caller still has to supply the
	// destination bank code. This branch is unreachable when Pakailink owns the
	// route (resolver always picks INTERBANK for pakailink), but kept defensive.
	return nil, newTransferError(503, "DISBURSEMENT_UNAVAILABLE", "PakaiLink intrabank inquiry not supported", nil)
}

func (a *pakailinkTransferAdapter) InterbankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error) {
	return a.transfer(ctx, req)
}

func (a *pakailinkTransferAdapter) IntrabankTransfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error) {
	return a.transfer(ctx, req)
}

func (a *pakailinkTransferAdapter) transfer(ctx context.Context, req bnc.TransferRequest) (*bnc.TransferResponse, error) {
	resp, err := a.client.TransferBank(ctx, pakailink.TransferBankRequest{
		PartnerReferenceNo:       req.PartnerReferenceNo,
		BeneficiaryAccountNumber: req.BeneficiaryAccountNo,
		BeneficiaryBankCode:      req.BeneficiaryBankCode,
		Amount:                   req.Amount,
		CallbackURL:              a.callbackURL,
		Remark:                   req.Remark,
	})
	if err != nil {
		return nil, pakailinkAsBNCError(err)
	}
	return &bnc.TransferResponse{
		ResponseCode:         resp.ResponseCode,
		ResponseMessage:      resp.ResponseMessage,
		ReferenceNo:          resp.ReferenceNo,
		PartnerReferenceNo:   resp.PartnerReferenceNo,
		Amount:               bnc.Amount{Value: resp.Amount.Value, Currency: resp.Amount.Currency},
		BeneficiaryAccountNo: resp.BeneficiaryAccountNumber,
		BeneficiaryBankCode:  req.BeneficiaryBankCode,
		TransactionDate:      "",
		RawResponse:          resp.RawResponse,
	}, nil
}

func (a *pakailinkTransferAdapter) TransferStatus(ctx context.Context, req bnc.TransferStatusRequest) (*bnc.TransferStatusResponse, error) {
	resp, err := a.client.TransferStatus(ctx, req.OriginalPartnerReferenceNo)
	if err != nil {
		return nil, pakailinkAsBNCError(err)
	}
	return &bnc.TransferStatusResponse{
		ResponseCode:               resp.ResponseCode,
		ResponseMessage:            resp.ResponseMessage,
		OriginalReferenceNo:        resp.OriginalReferenceNo,
		OriginalPartnerReferenceNo: resp.OriginalPartnerReferenceNo,
		ServiceCode:                resp.ServiceCode,
		TransactionDate:            resp.TransactionDate,
		Amount:                     bnc.Amount{Value: resp.Amount.Value, Currency: resp.Amount.Currency},
		BeneficiaryAccountNo:       resp.BeneficiaryAccountNumber,
		BeneficiaryBankCode:        resp.BeneficiaryBankCode,
		LatestTransactionStatus:    resp.LatestTransactionStatus,
		TransactionStatusDesc:      resp.LatestTransactionStatusDesc.English,
		RawResponse:                resp.RawResponse,
	}, nil
}

// PakailinkFeeFromResponse extracts the feeAmount returned by Service 43 so
// callers can store the actual fee charged by PakaiLink (rather than relying
// on a hardcoded constant).
func PakailinkFeeFromResponse(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var p struct {
		FeeAmount pakailink.Amount `json:"feeAmount"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0
	}
	v, _ := pakailink.ParseWebhookAmount(p.FeeAmount)
	return v
}

// pakailinkAsBNCError translates pakailink.APIError to bnc.APIError so existing
// TransferService error mapping (which only knows the BNC type) continues to
// work without refactoring every call site.
func pakailinkAsBNCError(err error) error {
	var apiErr *pakailink.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	return &bnc.APIError{
		HTTPStatus:      apiErr.HTTPStatus,
		ResponseCode:    apiErr.ResponseCode,
		ResponseMessage: apiErr.ResponseMessage,
		RawResponse:     apiErr.RawResponse,
	}
}

