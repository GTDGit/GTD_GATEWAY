package service

import (
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// ----------------------------------------------------------------------------
// Generators
// ----------------------------------------------------------------------------

// genPaymentType draws one of the four canonical payment types.
func genPaymentType(t *rapid.T) models.PaymentType {
	return rapid.SampledFrom([]models.PaymentType{
		models.PaymentTypeVA,
		models.PaymentTypeEwallet,
		models.PaymentTypeQRIS,
		models.PaymentTypeRetail,
	}).Draw(t, "paymentType")
}

// genPaymentCode draws a representative method code per the matrix.
func genPaymentCode(t *rapid.T) string {
	return rapid.SampledFrom([]string{
		"MPM", "CPM", "ALFAMART", "INDOMARET",
		"DANA", "GOPAY", "OVO", "LINKAJA", "SHOPEEPAY", "ASTRAPAY",
		"014", "002", "009", "008",
	}).Draw(t, "paymentCode")
}

// genPaymentStatus draws any payment status.
func genPaymentStatus(t *rapid.T) models.PaymentStatus {
	return rapid.SampledFrom([]models.PaymentStatus{
		models.PaymentStatusPending,
		models.PaymentStatusPaid,
		models.PaymentStatusExpired,
		models.PaymentStatusCancelled,
		models.PaymentStatusFailed,
		models.PaymentStatusRefunded,
		models.PaymentStatusPartialRefund,
	}).Draw(t, "status")
}

// genFeePaidBy draws a fee bearer.
func genFeePaidBy(t *rapid.T) models.FeePaidBy {
	return rapid.SampledFrom([]models.FeePaidBy{
		models.FeePaidByMerchant,
		models.FeePaidByCustomer,
	}).Draw(t, "feePaidBy")
}

// genProvider draws an internal provider. The provider must NEVER leak into the
// marshaled Standard_Response, so we randomize it to make the absence robust.
func genProvider(t *rapid.T) models.PaymentProvider {
	return rapid.SampledFrom([]models.PaymentProvider{
		models.ProviderPakailink,
		models.ProviderDanaDirect,
		models.ProviderMidtrans,
		models.ProviderXendit,
		models.ProviderOVODirect,
	}).Draw(t, "provider")
}

// genPayment builds a fully populated persisted payment with random values.
func genPayment(t *rapid.T) *models.Payment {
	feePaidBy := genFeePaidBy(t)
	subtotal := rapid.Int64Range(0, 1_000_000_000).Draw(t, "subtotal")
	fee := rapid.Int64Range(0, 1_000_000).Draw(t, "fee")
	total := subtotal
	if feePaidBy == models.FeePaidByCustomer {
		total = subtotal + fee
	}
	providerRef := rapid.StringMatching(`[A-Za-z0-9]{0,20}`).Draw(t, "providerRef")

	p := &models.Payment{
		PaymentID:   rapid.StringMatching(`PAY-[0-9]{8}-[0-9]{6}`).Draw(t, "id"),
		ReferenceID: rapid.StringMatching(`[A-Za-z0-9_-]{1,24}`).Draw(t, "referenceId"),
		PaymentType: genPaymentType(t),
		PaymentCode: genPaymentCode(t),
		Provider:    genProvider(t),
		Amount:      subtotal,
		Fee:         fee,
		TotalAmount: total,
		FeePaidBy:   feePaidBy,
		Status:      genPaymentStatus(t),
	}
	if providerRef != "" {
		p.ProviderRef = &providerRef
	}
	return p
}

// ----------------------------------------------------------------------------
// Property 3 (gateway side)
// ----------------------------------------------------------------------------

// Feature: payment-provider-enhancements, Property 3: For any persisted payment, the
// marshaled create/get response contains a nested paymentMethod{type,code} and a nested
// amount{subtotal,fee,total}, a feePaidBy field, and contains NO flat
// paymentType/paymentCode/totalAmount/paymentId keys and NO provider/providerRef key.
//
// Validates: Requirements 4.7, 14.2
func TestProperty3_GatewayStandardResponseShape(t *testing.T) {
	svc := &PaymentService{}

	rapid.Check(t, func(t *rapid.T) {
		p := genPayment(t)

		resp := svc.buildResponse(p)
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}

		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		// Nested paymentMethod{type,code}.
		pmRaw, ok := obj["paymentMethod"]
		if !ok {
			t.Fatalf("response missing nested paymentMethod: %s", raw)
		}
		var pm map[string]json.RawMessage
		if err := json.Unmarshal(pmRaw, &pm); err != nil {
			t.Fatalf("paymentMethod is not an object: %v", err)
		}
		if _, ok := pm["type"]; !ok {
			t.Fatalf("paymentMethod missing type: %s", pmRaw)
		}
		if _, ok := pm["code"]; !ok {
			t.Fatalf("paymentMethod missing code: %s", pmRaw)
		}

		// Nested amount{subtotal,fee,total}.
		amtRaw, ok := obj["amount"]
		if !ok {
			t.Fatalf("response missing nested amount: %s", raw)
		}
		var amt map[string]json.RawMessage
		if err := json.Unmarshal(amtRaw, &amt); err != nil {
			t.Fatalf("amount is not an object: %v", err)
		}
		for _, k := range []string{"subtotal", "fee", "total"} {
			if _, ok := amt[k]; !ok {
				t.Fatalf("amount missing %s: %s", k, amtRaw)
			}
		}

		// feePaidBy field present and equals the value used on the transaction.
		fpbRaw, ok := obj["feePaidBy"]
		if !ok {
			t.Fatalf("response missing feePaidBy: %s", raw)
		}
		var fpb string
		if err := json.Unmarshal(fpbRaw, &fpb); err != nil {
			t.Fatalf("feePaidBy not a string: %v", err)
		}
		if fpb != string(p.FeePaidBy) {
			t.Fatalf("feePaidBy mismatch: got %q want %q", fpb, p.FeePaidBy)
		}

		// No flat keys.
		for _, forbidden := range []string{"paymentType", "paymentCode", "totalAmount", "paymentId"} {
			if _, found := obj[forbidden]; found {
				t.Fatalf("response contains forbidden flat key %q: %s", forbidden, raw)
			}
		}

		// No provider name and no providerRef anywhere in the marshaled response.
		for _, forbidden := range []string{"provider", "providerRef"} {
			if _, found := obj[forbidden]; found {
				t.Fatalf("response contains forbidden provider key %q: %s", forbidden, raw)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Task 14.4: feePaidBy and id pass-through (unit test)
// ----------------------------------------------------------------------------

// TestBuildResponse_ForwardsFeePaidByAndID asserts the gateway forwards the
// feePaidBy value and preserves the id (public payment id) from Payment_API
// without modification, for both merchant and customer bearers.
//
// Validates: Requirements 2.7, 5.3
func TestBuildResponse_ForwardsFeePaidByAndID(t *testing.T) {
	svc := &PaymentService{}

	tests := []struct {
		name      string
		paymentID string
		feePaidBy models.FeePaidBy
		subtotal  int64
		fee       int64
		total     int64
	}{
		{
			name:      "merchant bearer preserves id and feePaidBy",
			paymentID: "PAY-20260101-000123",
			feePaidBy: models.FeePaidByMerchant,
			subtotal:  10000,
			fee:       500,
			total:     10000, // merchant -> total = subtotal
		},
		{
			name:      "customer bearer preserves id and feePaidBy",
			paymentID: "PAY-20260102-987654",
			feePaidBy: models.FeePaidByCustomer,
			subtotal:  10000,
			fee:       500,
			total:     10500, // customer -> total = subtotal + fee
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &models.Payment{
				PaymentID:   tc.paymentID,
				ReferenceID: "ref-123",
				PaymentType: models.PaymentTypeEwallet,
				PaymentCode: "OVO",
				Provider:    models.ProviderPakailink,
				Amount:      tc.subtotal,
				Fee:         tc.fee,
				TotalAmount: tc.total,
				FeePaidBy:   tc.feePaidBy,
				Status:      models.PaymentStatusPending,
			}

			resp := svc.buildResponse(p)

			// id preserved unchanged from Payment_API.
			if resp.ID != tc.paymentID {
				t.Errorf("id not preserved: got %q want %q", resp.ID, tc.paymentID)
			}

			// feePaidBy forwarded unchanged.
			if resp.FeePaidBy != string(tc.feePaidBy) {
				t.Errorf("feePaidBy not forwarded: got %q want %q", resp.FeePaidBy, tc.feePaidBy)
			}

			// Confirm the same survives JSON marshaling (what the client sees).
			raw, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var obj struct {
				ID        string `json:"id"`
				FeePaidBy string `json:"feePaidBy"`
			}
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if obj.ID != tc.paymentID {
				t.Errorf("marshaled id not preserved: got %q want %q", obj.ID, tc.paymentID)
			}
			if obj.FeePaidBy != string(tc.feePaidBy) {
				t.Errorf("marshaled feePaidBy not forwarded: got %q want %q", obj.FeePaidBy, tc.feePaidBy)
			}
		})
	}
}
