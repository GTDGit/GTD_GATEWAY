package sse

import (
	"encoding/json"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// stringGen produces strings that exercise the interesting parts of the input
// space for JSON round-tripping: empty strings, arbitrary unicode (including
// code points outside the BMP and JSON-significant characters), and long
// strings.
func stringGen() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Just(""),
		// Arbitrary unicode strings (rapid.String covers the full rune space
		// including multi-byte/unicode and JSON-escape-significant runes).
		rapid.String(),
		// Long strings to exercise larger payloads.
		rapid.StringN(256, 1024, 1024),
	)
}

// optStringGen produces a *string that is nil roughly half the time and
// otherwise points at a value drawn from stringGen, exercising the omitempty
// pointer fields in both nil and set states.
func optStringGen() *rapid.Generator[*string] {
	return rapid.Custom(func(t *rapid.T) *string {
		if rapid.Bool().Draw(t, "nil") {
			return nil
		}
		s := stringGen().Draw(t, "val")
		return &s
	})
}

// optIntGen produces a *int that is nil roughly half the time and otherwise
// points at an arbitrary int (including negatives and zero).
func optIntGen() *rapid.Generator[*int] {
	return rapid.Custom(func(t *rapid.T) *int {
		if rapid.Bool().Draw(t, "nil") {
			return nil
		}
		v := rapid.Int().Draw(t, "val")
		return &v
	})
}

// timeGen produces timestamps at the precision JSON preserves for time.Time.
// time.Time marshals to RFC3339Nano (nanosecond precision, UTC offset here)
// and decodes back without a monotonic-clock reading or a *time.Location
// pointer, so generating UTC times built from unix seconds + nanoseconds
// yields values that survive an exact round-trip.
func timeGen() *rapid.Generator[time.Time] {
	return rapid.Custom(func(t *rapid.T) time.Time {
		// Bound the seconds to a wide-but-sane range around the epoch
		// (roughly years 1970..2262) to avoid overflow while still covering
		// past/future timestamps.
		sec := rapid.Int64Range(0, 9_000_000_000).Draw(t, "sec")
		nsec := rapid.Int64Range(0, 999_999_999).Draw(t, "nsec")
		return time.Unix(sec, nsec).UTC()
	})
}

func transactionEventGen() *rapid.Generator[TransactionEvent] {
	return rapid.Custom(func(t *rapid.T) TransactionEvent {
		eventType := rapid.SampledFrom([]EventType{
			EventTransactionCreated,
			EventTransactionStatusChanged,
		}).Draw(t, "event")
		return TransactionEvent{
			Event:         eventType,
			TransactionID: stringGen().Draw(t, "transactionId"),
			ReferenceID:   stringGen().Draw(t, "referenceId"),
			CustomerNo:    stringGen().Draw(t, "customerNo"),
			SkuCode:       stringGen().Draw(t, "skuCode"),
			Type:          stringGen().Draw(t, "type"),
			Status:        stringGen().Draw(t, "status"),
			ProviderCode:  optStringGen().Draw(t, "providerCode"),
			FailedReason:  optStringGen().Draw(t, "failedReason"),
			Amount:        optIntGen().Draw(t, "amount"),
			BuyPrice:      optIntGen().Draw(t, "buyPrice"),
			SellPrice:     optIntGen().Draw(t, "sellPrice"),
			Timestamp:     timeGen().Draw(t, "timestamp"),
		}
	})
}

func paymentEventGen() *rapid.Generator[PaymentEvent] {
	return rapid.Custom(func(t *rapid.T) PaymentEvent {
		eventType := rapid.SampledFrom([]EventType{
			EventPaymentCreated,
			EventPaymentStatusChanged,
		}).Draw(t, "event")
		return PaymentEvent{
			Event:       eventType,
			PaymentID:   stringGen().Draw(t, "paymentId"),
			ReferenceID: stringGen().Draw(t, "referenceId"),
			ClientID:    rapid.Int().Draw(t, "clientId"),
			Type:        stringGen().Draw(t, "type"),
			Status:      stringGen().Draw(t, "status"),
			Provider:    stringGen().Draw(t, "provider"),
			Amount:      rapid.Int64().Draw(t, "amount"),
			TotalAmount: rapid.Int64().Draw(t, "totalAmount"),
			Timestamp:   timeGen().Draw(t, "timestamp"),
		}
	})
}

// optStringEqual compares two optional string pointers semantically: both nil,
// or both non-nil with equal values.
func optStringEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// optIntEqual compares two optional int pointers semantically.
func optIntEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Feature: gateway-extraction-rds-migration, Property 1: Domain_Event serialization round-trip
func TestProperty_DomainEventSerializationRoundTrip(t *testing.T) {
	// A single property-based test covering BOTH Domain_Event variants
	// (TransactionEvent and PaymentEvent). A discriminator picks which event
	// type to exercise on each iteration. rapid runs >= 100 iterations by
	// default (configurable via -rapid.checks; the default is 100).
	rapid.Check(t, func(t *rapid.T) {
		isPayment := rapid.Bool().Draw(t, "isPayment")

		if isPayment {
			original := paymentEventGen().Draw(t, "paymentEvent")

			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal payment event: %v", err)
			}
			var decoded PaymentEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal payment event: %v", err)
			}

			// Compare timestamps semantically (.Equal) to avoid false negatives
			// from monotonic-clock readings or *time.Location identity.
			if !original.Timestamp.Equal(decoded.Timestamp) {
				t.Fatalf("payment timestamp mismatch: original=%v decoded=%v", original.Timestamp, decoded.Timestamp)
			}
			// Compare the remaining fields by normalizing the timestamp, then
			// asserting struct equality.
			origNorm := original
			decNorm := decoded
			origNorm.Timestamp = time.Time{}
			decNorm.Timestamp = time.Time{}
			if origNorm != decNorm {
				t.Fatalf("payment event mismatch:\noriginal=%#v\ndecoded =%#v", original, decoded)
			}
			return
		}

		original := transactionEventGen().Draw(t, "transactionEvent")

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal transaction event: %v", err)
		}
		var decoded TransactionEvent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal transaction event: %v", err)
		}

		if !original.Timestamp.Equal(decoded.Timestamp) {
			t.Fatalf("transaction timestamp mismatch: original=%v decoded=%v", original.Timestamp, decoded.Timestamp)
		}
		// TransactionEvent contains pointer fields, so it is not comparable with
		// ==; compare each field explicitly, using semantic pointer equality.
		if original.Event != decoded.Event ||
			original.TransactionID != decoded.TransactionID ||
			original.ReferenceID != decoded.ReferenceID ||
			original.CustomerNo != decoded.CustomerNo ||
			original.SkuCode != decoded.SkuCode ||
			original.Type != decoded.Type ||
			original.Status != decoded.Status ||
			!optStringEqual(original.ProviderCode, decoded.ProviderCode) ||
			!optStringEqual(original.FailedReason, decoded.FailedReason) ||
			!optIntEqual(original.Amount, decoded.Amount) ||
			!optIntEqual(original.BuyPrice, decoded.BuyPrice) ||
			!optIntEqual(original.SellPrice, decoded.SellPrice) {
			t.Fatalf("transaction event mismatch:\noriginal=%#v\ndecoded =%#v", original, decoded)
		}
	})
}
