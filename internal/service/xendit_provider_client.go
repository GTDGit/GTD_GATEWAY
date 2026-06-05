package service

import (
	"context"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/xendit"
)

// XenditProviderClient implements PaymentProviderClient using Xendit Payment
// Request API for retail (Indomaret/Alfamart) channels.
type XenditProviderClient struct {
	client *xendit.Client
}

func NewXenditProviderClient(client *xendit.Client) *XenditProviderClient {
	return &XenditProviderClient{client: client}
}

func (p *XenditProviderClient) Code() models.PaymentProvider {
	return models.ProviderXendit
}

func (p *XenditProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	if req.Type != models.PaymentTypeRetail {
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "Xendit adapter only supports retail payments", nil)
	}
	channel := strings.ToUpper(strings.TrimSpace(method.Code))
	create := xendit.PaymentRequestCreate{
		ReferenceID:   req.PartnerRef,
		Type:          "PAY",
		Country:       "ID",
		Currency:      "IDR",
		ChannelCode:   channel,
		RequestAmount: req.TotalAmount,
		ChannelProperties: xendit.PaymentRequestChannelProperties{
			PayerName: firstNonEmpty(req.CustomerName, "Customer"),
			ExpiresAt: formatXenditExpiry(req.ExpiredAt),
		},
		Description: req.Description,
	}
	resp, err := p.client.CreatePaymentRequest(ctx, create)
	if err != nil {
		return nil, mapXenditError(err)
	}
	retailName := method.Name
	if retailName == "" {
		retailName = channel
	}
	norm := PaymentDetailNormalized{
		Provider:            string(models.ProviderXendit),
		RetailName:          retailName,
		PaymentCode:         resp.ChannelProperties.PaymentCode,
		ProviderReferenceNo: resp.PaymentRequestID,
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.PaymentRequestID,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *XenditProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return &PaymentInquiryResult{Status: payment.Status}, nil
	}
	resp, err := p.client.GetPaymentRequest(ctx, *payment.ProviderRef)
	if err != nil {
		return nil, mapXenditError(err)
	}
	return &PaymentInquiryResult{
		Status:      mapXenditStatus(resp.Status),
		ProviderRef: resp.PaymentRequestID,
		PaidAmount:  payment.Amount,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *XenditProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return &PaymentCancelResult{Cancelled: true}, nil
	}
	resp, err := p.client.CancelPaymentRequest(ctx, *payment.ProviderRef)
	if err != nil {
		return nil, mapXenditError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func (p *XenditProviderClient) RefundPayment(ctx context.Context, payment *models.Payment, refund *models.Refund) (*PaymentRefundResult, error) {
	if payment.ProviderRef == nil || *payment.ProviderRef == "" {
		return nil, newPaymentError(400, "PROVIDER_REF_MISSING", "Cannot refund without provider reference", nil)
	}
	create := xendit.RefundCreate{
		Amount:      refund.Amount,
		ReferenceID: refund.RefundID,
		Reason:      firstNonEmpty(refund.Reason, "Refund"),
	}
	resp, err := p.client.CreateRefund(ctx, *payment.ProviderRef, create)
	if err != nil {
		return nil, mapXenditError(err)
	}
	return &PaymentRefundResult{
		ProviderRef: resp.RefundID,
		Succeeded:   true,
		RawResponse: resp.RawResponse,
	}, nil
}

func formatXenditExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func mapXenditStatus(status string) models.PaymentStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case xendit.StatusSucceeded:
		return models.PaymentStatusPaid
	case xendit.StatusExpired:
		return models.PaymentStatusExpired
	case xendit.StatusCanceled:
		return models.PaymentStatusCancelled
	case xendit.StatusFailed:
		return models.PaymentStatusFailed
	default:
		return models.PaymentStatusPending
	}
}

func mapXenditError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*xendit.APIError)
	if ok {
		if apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.Message, "Provider rejected request"), err)
		}
		if apiErr.HTTPStatus >= 500 {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
