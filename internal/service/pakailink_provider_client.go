package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/pakailink"
)

// PakailinkProviderClient wraps pkg/pakailink to implement PaymentProviderClient
// for both VA and QRIS MPM flows.
type PakailinkProviderClient struct {
	client      *pakailink.Client
	callbackURL string
}

func NewPakailinkProviderClient(client *pakailink.Client, callbackURL string) *PakailinkProviderClient {
	return &PakailinkProviderClient{client: client, callbackURL: callbackURL}
}

func (p *PakailinkProviderClient) Code() models.PaymentProvider {
	return models.ProviderPakailink
}

func (p *PakailinkProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	switch req.Type {
	case models.PaymentTypeVA:
		return p.createVA(ctx, method, req)
	case models.PaymentTypeQRIS:
		return p.createQRIS(ctx, method, req)
	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Pakailink does not support this payment type", nil)
	}
}

func (p *PakailinkProviderClient) createVA(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	bankCode := strings.TrimSpace(method.Code)
	customerNo := req.PartnerRef
	vaReq := pakailink.CreateVARequest{
		PartnerReferenceNo:  req.PartnerRef,
		CustomerNo:          customerNo,
		VirtualAccountName:  firstNonEmpty(req.CustomerName, "Customer"),
		VirtualAccountEmail: req.CustomerEmail,
		VirtualAccountPhone: req.CustomerPhone,
		TotalAmount:         req.TotalAmount,
		BankCode:            bankCode,
		CallbackURL:         firstNonEmpty(req.CallbackURL, p.callbackURL),
		ExpiredDate:         formatPakailinkExpiry(req.ExpiredAt),
	}
	resp, err := p.client.CreateVA(ctx, vaReq)
	if err != nil {
		return nil, mapPakailinkError(err)
	}
	norm := PaymentDetailNormalized{
		BankCode:            bankCode,
		BankName:            method.Name,
		VANumber:            resp.VirtualAccountData.VirtualAccountNo,
		AccountName:         resp.VirtualAccountData.VirtualAccountName,
		ProviderReferenceNo: resp.VirtualAccountData.PartnerReferenceNo,
		Provider:            string(models.ProviderPakailink),
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.VirtualAccountData.PartnerReferenceNo,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *PakailinkProviderClient) createQRIS(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	qrReq := pakailink.GenerateQRRequest{
		PartnerReferenceNo: req.PartnerRef,
		Amount:             req.TotalAmount,
		MerchantName:       firstNonEmpty(req.CustomerName, "GTD Gateway"),
		Description:        req.Description,
		CallbackURL:        firstNonEmpty(req.CallbackURL, p.callbackURL),
		ExpiredDate:        formatPakailinkExpiry(req.ExpiredAt),
	}
	resp, err := p.client.GenerateQRMPM(ctx, qrReq)
	if err != nil {
		return nil, mapPakailinkError(err)
	}
	qrString := resp.QRContent
	if qrString == "" {
		if v, ok := resp.AdditionalInfo["paymentQrString"].(string); ok {
			qrString = v
		}
	}
	qrImage := ""
	if v, ok := resp.AdditionalInfo["qrImageUrl"].(string); ok {
		qrImage = v
	}
	norm := PaymentDetailNormalized{
		QRString:            qrString,
		QRImageURL:          qrImage,
		ProviderReferenceNo: firstNonEmpty(resp.ReferenceNo, resp.PartnerReferenceNo),
		Provider:            string(models.ProviderPakailink),
	}
	return &PaymentCreateResponse{
		ProviderRef: firstNonEmpty(resp.ReferenceNo, resp.PartnerReferenceNo),
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *PakailinkProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	switch payment.PaymentType {
	case models.PaymentTypeVA:
		resp, err := p.client.InquiryVA(ctx, payment.PaymentID)
		if err != nil {
			return nil, mapPakailinkError(err)
		}
		status := mapPakailinkTransactionStatus(resp.LatestTransactionStatus)
		amount, _ := pakailink.ParseWebhookAmount(resp.Amount)
		return &PaymentInquiryResult{
			Status:      status,
			ProviderRef: firstNonEmpty(resp.OriginalReferenceNo, resp.OriginalPartnerReferenceNo),
			PaidAmount:  amount,
			RawResponse: resp.RawResponse,
		}, nil
	case models.PaymentTypeQRIS:
		resp, err := p.client.InquiryQR(ctx, payment.PaymentID)
		if err != nil {
			return nil, mapPakailinkError(err)
		}
		status := mapPakailinkTransactionStatus(resp.LatestTransactionStatus)
		amount, _ := pakailink.ParseWebhookAmount(resp.Amount)
		return &PaymentInquiryResult{
			Status:      status,
			ProviderRef: firstNonEmpty(resp.OriginalReferenceNo, resp.OriginalPartnerReferenceNo),
			PaidAmount:  amount,
			RawResponse: resp.RawResponse,
		}, nil
	}
	return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Inquiry not supported for this type", nil)
}

func (p *PakailinkProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	if payment.PaymentType != models.PaymentTypeVA {
		// QRIS MPM cannot be cancelled once generated; treat as soft cancel locally.
		return &PaymentCancelResult{Cancelled: true}, nil
	}
	norm := normalizedPaymentDetail(payment)
	if norm.VANumber == "" {
		return &PaymentCancelResult{Cancelled: true}, nil
	}
	req := pakailink.DeleteVARequest{
		PartnerReferenceNo: payment.PaymentID,
		CustomerNo:         payment.PaymentID,
		VirtualAccountNo:   norm.VANumber,
	}
	resp, err := p.client.DeleteVA(ctx, req)
	if err != nil {
		return nil, mapPakailinkError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func (p *PakailinkProviderClient) RefundPayment(ctx context.Context, payment *models.Payment, refund *models.Refund) (*PaymentRefundResult, error) {
	// Pakailink does not expose refund APIs for VA/QRIS — document as manual ops.
	return nil, newPaymentError(501, "REFUND_NOT_SUPPORTED", "Pakailink does not support automated refunds", nil)
}

func formatPakailinkExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02T15:04:05+07:00")
}

func mapPakailinkTransactionStatus(code string) models.PaymentStatus {
	switch strings.TrimSpace(code) {
	case pakailink.StatusSuccess:
		return models.PaymentStatusPaid
	case pakailink.StatusCancelled:
		return models.PaymentStatusCancelled
	case pakailink.StatusFailed:
		return models.PaymentStatusFailed
	case "07":
		return models.PaymentStatusExpired
	default:
		return models.PaymentStatusPending
	}
}

func mapPakailinkError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *pakailink.APIError
	if asPakailinkAPIError(err, &apiErr) {
		if strings.HasPrefix(apiErr.ResponseCode, "4") {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmptyStr(apiErr.ResponseMessage, "Provider rejected request"), err)
		}
		if strings.HasPrefix(apiErr.ResponseCode, "5") {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}

func asPakailinkAPIError(err error, target **pakailink.APIError) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*pakailink.APIError)
	if !ok {
		return false
	}
	*target = e
	return true
}

func normalizedPaymentDetail(payment *models.Payment) PaymentDetailNormalized {
	var out PaymentDetailNormalized
	if len(payment.PaymentDetail) == 0 {
		return out
	}
	_ = json.Unmarshal(payment.PaymentDetail, &out)
	return out
}

func firstNonEmptyStr(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vs ...string) string {
	return firstNonEmptyStr(vs...)
}
