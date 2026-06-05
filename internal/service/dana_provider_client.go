package service

import (
	"context"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
	"github.com/GTDGit/gtd_gateway/pkg/dana"
)

// DanaProviderClient wraps pkg/dana to implement PaymentProviderClient for
// DANA e-wallet and QRIS MPM via DANA Direct.
type DanaProviderClient struct {
	client          *dana.Client
	notificationURL string
	returnURL       string
}

func NewDanaProviderClient(client *dana.Client, notificationURL, returnURL string) *DanaProviderClient {
	return &DanaProviderClient{client: client, notificationURL: notificationURL, returnURL: returnURL}
}

func (p *DanaProviderClient) Code() models.PaymentProvider {
	return models.ProviderDanaDirect
}

func (p *DanaProviderClient) CreatePayment(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest) (*PaymentCreateResponse, error) {
	switch req.Type {
	case models.PaymentTypeEwallet:
		return p.createOrder(ctx, method, req, dana.PayMethodBalance, "")
	case models.PaymentTypeQRIS:
		return p.createOrder(ctx, method, req, dana.PayMethodNetworkPay, dana.PayOptionQRIS)
	default:
		return nil, newPaymentError(400, "UNSUPPORTED_PAYMENT_TYPE", "DANA does not support this payment type", nil)
	}
}

func (p *DanaProviderClient) createOrder(ctx context.Context, method *models.PaymentMethod, req *PaymentCreateRequest, payMethod, payOption string) (*PaymentCreateResponse, error) {
	order := dana.CreateOrderRequest{
		PartnerReferenceNo: req.PartnerRef,
		ExternalStoreID:    req.PartnerRef,
		Amount:             req.TotalAmount,
		ValidUpTo:          formatDanaExpiry(req.ExpiredAt),
		NotificationURL:    firstNonEmpty(req.CallbackURL, p.notificationURL),
		ReturnURL:          firstNonEmpty(req.ReturnURL, p.returnURL),
		PayMethod:          payMethod,
		PayOption:          payOption,
		OrderTitle:         firstNonEmpty(req.Description, method.Name),
	}
	resp, err := p.client.CreateOrder(ctx, order)
	if err != nil {
		return nil, mapDanaError(err)
	}
	norm := PaymentDetailNormalized{
		Provider:            string(models.ProviderDanaDirect),
		ProviderReferenceNo: resp.ReferenceNo,
	}
	if req.Type == models.PaymentTypeQRIS {
		norm.QRString = resp.PaymentCode()
	} else {
		norm.CheckoutURL = resp.CheckoutURL
		norm.MobileWebURL = resp.WebRedirectURL
		norm.Deeplink = resp.DeeplinkURL
	}
	return &PaymentCreateResponse{
		ProviderRef: resp.ReferenceNo,
		Normalized:  norm,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *DanaProviderClient) InquiryPayment(ctx context.Context, payment *models.Payment) (*PaymentInquiryResult, error) {
	resp, err := p.client.InquiryOrder(ctx, payment.PaymentID)
	if err != nil {
		return nil, mapDanaError(err)
	}
	status := mapDanaTransactionStatus(resp.LatestTransactionStatus)
	amount, _ := dana.ParseWebhookAmount(resp.Amount)
	return &PaymentInquiryResult{
		Status:      status,
		ProviderRef: resp.OriginalReferenceNo,
		PaidAmount:  amount,
		RawResponse: resp.RawResponse,
	}, nil
}

func (p *DanaProviderClient) CancelPayment(ctx context.Context, payment *models.Payment, reason string) (*PaymentCancelResult, error) {
	req := dana.CancelOrderRequest{
		PartnerReferenceNo: payment.PaymentID,
		Reason:             firstNonEmpty(reason, "Customer cancellation"),
	}
	resp, err := p.client.CancelOrder(ctx, req)
	if err != nil {
		return nil, mapDanaError(err)
	}
	return &PaymentCancelResult{Cancelled: true, RawResponse: resp.RawResponse}, nil
}

func (p *DanaProviderClient) RefundPayment(ctx context.Context, payment *models.Payment, refund *models.Refund) (*PaymentRefundResult, error) {
	req := dana.RefundRequest{
		OriginalPartnerReference: payment.PaymentID,
		PartnerRefundNo:          refund.RefundID,
		RefundAmount:             refund.Amount,
		Reason:                   firstNonEmpty(refund.Reason, "Refund"),
	}
	if payment.ProviderRef != nil {
		req.OriginalReferenceNo = *payment.ProviderRef
	}
	resp, err := p.client.Refund(ctx, req)
	if err != nil {
		return nil, mapDanaError(err)
	}
	return &PaymentRefundResult{
		ProviderRef: resp.RefundNo,
		Succeeded:   true,
		RawResponse: resp.RawResponse,
	}, nil
}

func formatDanaExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02T15:04:05+07:00")
}

func mapDanaTransactionStatus(code string) models.PaymentStatus {
	switch strings.TrimSpace(code) {
	case "00":
		return models.PaymentStatusPaid
	case "05":
		return models.PaymentStatusCancelled
	case "06":
		return models.PaymentStatusFailed
	case "07":
		return models.PaymentStatusExpired
	default:
		return models.PaymentStatusPending
	}
}

func mapDanaError(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := err.(*dana.APIError)
	if ok {
		if strings.HasPrefix(apiErr.ResponseCode, "4") {
			return newPaymentError(400, "PROVIDER_REQUEST_REJECTED", firstNonEmpty(apiErr.ResponseMessage, "Provider rejected request"), err)
		}
		if strings.HasPrefix(apiErr.ResponseCode, "5") {
			return newPaymentError(503, "PROVIDER_UNAVAILABLE", "Payment provider temporarily unavailable", err)
		}
	}
	return newPaymentError(502, "PROVIDER_ERROR", "Payment provider error", err)
}
