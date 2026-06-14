package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// QRISInfo holds the fields extracted from an EMVCo QRIS payload string.
// storeId is intentionally absent: it is never encoded in the QR and must be
// entered manually (it is the key used to identify a merchant on webhook).
type QRISInfo struct {
	NMID                 string `json:"nmid"`
	MerchantName         string `json:"merchantName"`
	MerchantCity         string `json:"merchantCity"`
	MerchantCategoryCode string `json:"merchantCategoryCode"`
	TerminalID           string `json:"terminalId"`
	IsStatic             bool   `json:"isStatic"`
}

type emvTag struct {
	id    string
	value string
}

// ParseQRIS walks an EMVCo Merchant-Presented-Mode QR string and extracts the
// QRIS domestic identifiers. It tolerates unknown tags and only fails when the
// TLV framing itself is malformed (bad length, truncated value).
func ParseQRIS(qr string) (QRISInfo, error) {
	qr = strings.TrimSpace(qr)
	if qr == "" {
		return QRISInfo{}, fmt.Errorf("qris: empty string")
	}

	tags, err := walkTLV(qr)
	if err != nil {
		return QRISInfo{}, err
	}

	info := QRISInfo{}
	for _, t := range tags {
		switch t.id {
		case "01":
			info.IsStatic = t.value == "11"
		case "51":
			sub, err := walkTLV(t.value)
			if err != nil {
				return QRISInfo{}, fmt.Errorf("qris: tag 51 sub-tlv: %w", err)
			}
			for _, s := range sub {
				if s.id == "02" {
					info.NMID = s.value
				}
			}
		case "52":
			info.MerchantCategoryCode = t.value
		case "59":
			info.MerchantName = t.value
		case "60":
			info.MerchantCity = t.value
		case "62":
			sub, err := walkTLV(t.value)
			if err != nil {
				return QRISInfo{}, fmt.Errorf("qris: tag 62 sub-tlv: %w", err)
			}
			for _, s := range sub {
				if s.id == "07" {
					info.TerminalID = s.value
				}
			}
		}
	}

	if info.NMID == "" {
		return info, fmt.Errorf("qris: NMID (tag 51 subfield 02) not found")
	}
	return info, nil
}

func walkTLV(s string) ([]emvTag, error) {
	var out []emvTag
	for i := 0; i < len(s); {
		if i+4 > len(s) {
			return nil, fmt.Errorf("qris: truncated tag header at offset %d", i)
		}
		id := s[i : i+2]
		lenStr := s[i+2 : i+4]
		length, err := strconv.Atoi(lenStr)
		if err != nil {
			return nil, fmt.Errorf("qris: invalid length %q at offset %d", lenStr, i)
		}
		start := i + 4
		end := start + length
		if end > len(s) {
			return nil, fmt.Errorf("qris: value for tag %s overruns input (need %d bytes)", id, length)
		}
		out = append(out, emvTag{id: id, value: s[start:end]})
		i = end
	}
	return out, nil
}
