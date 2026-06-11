package models

import "time"

// ----------------------------------------------------------------------------
// MethodProviderBinding mirrors the payment_method_providers table — the
// Method_Provider_Mapping that binds one canonical payment method to one
// provider with an explicit priority and health flags. The schema is owned by
// the api service (migration 000051_add_payment_method_providers); the gateway
// reads/writes the same shared table.
// ----------------------------------------------------------------------------

type MethodProviderBinding struct {
	ID                 int             `db:"id" json:"id"`
	PaymentMethodID    int             `db:"payment_method_id" json:"paymentMethodId"`
	Provider           PaymentProvider `db:"provider" json:"provider"`
	Priority           int             `db:"priority" json:"priority"`
	IsActive           bool            `db:"is_active" json:"isActive"`
	IsMaintenance      bool            `db:"is_maintenance" json:"isMaintenance"`
	MaintenanceMessage *string         `db:"maintenance_message" json:"maintenanceMessage,omitempty"`
	ProviderBankCode   *string         `db:"provider_bank_code" json:"providerBankCode,omitempty"`
	ProviderChannel    *string         `db:"provider_channel" json:"providerChannel,omitempty"`
	CreatedAt          time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time       `db:"updated_at" json:"updatedAt"`
}
