package service

import (
	"math/rand"
	"strings"
	"time"

	"github.com/GTDGit/gtd_gateway/internal/models"
)

// SandboxMapper handles mapping of client SKUs and customer numbers to Digiflazz test cases.
type SandboxMapper struct {
	rnd *rand.Rand
}

// NewSandboxMapper creates a new sandbox mapper.
func NewSandboxMapper() *SandboxMapper {
	return &SandboxMapper{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// TestSKUMapping represents a test case mapping.
type TestSKUMapping struct {
	TestSKU      string
	SuccessCustomer string
	FailCustomer    string
	PendingSuccessCustomer string
	PendingFailCustomer string
}

// GetTestMapping returns the appropriate test SKU and customer number for sandbox mode.
// It accepts client's original SKU and transaction type, and returns test data.
func (m *SandboxMapper) GetTestMapping(clientSKU string, trxType models.TransactionType) (testSKU string, testCustomerNo string) {
	// For prepaid transactions, always use xld10
	if trxType == models.TrxTypePrepaid {
		return m.getPrepaidMapping()
	}

	// For postpaid (inquiry/payment), map based on product category
	return m.getPostpaidMapping(clientSKU)
}

// getPrepaidMapping returns test SKU and random customer number for prepaid.
func (m *SandboxMapper) getPrepaidMapping() (string, string) {
	testSKU := "xld10"

	// Randomly choose one of the test customer numbers
	customers := []string{
		"087800001230", // Success
		"087800001232", // Fail
		"087800001233", // Pending -> Success
		"087800001234", // Pending -> Fail
	}

	testCustomerNo := customers[m.rnd.Intn(len(customers))]
	return testSKU, testCustomerNo
}

// getPostpaidMapping returns test SKU and customer number for postpaid based on category.
func (m *SandboxMapper) getPostpaidMapping(clientSKU string) (string, string) {
	// Detect category from client SKU
	skuLower := strings.ToLower(clientSKU)

	var mapping *TestSKUMapping

	// Map to appropriate test case
	switch {
	case strings.Contains(skuLower, "pln") && !strings.Contains(skuLower, "nontaglis"):
		mapping = &TestSKUMapping{
			TestSKU:               "pln",
			SuccessCustomer:       "530000000001",
			FailCustomer:          "530000000003",
			PendingSuccessCustomer: "630000000001",
			PendingFailCustomer:   "630000000006",
		}
	case strings.Contains(skuLower, "plnnontaglis"):
		mapping = &TestSKUMapping{
			TestSKU:               "plnnontaglist",
			SuccessCustomer:       "3225030005921",
			FailCustomer:          "3225030005922",
			PendingSuccessCustomer: "4225030005921",
			PendingFailCustomer:   "4225030005923",
		}
	case strings.Contains(skuLower, "pdam"):
		mapping = &TestSKUMapping{
			TestSKU:               "pdam",
			SuccessCustomer:       "1013226",
			FailCustomer:          "1013227",
			PendingSuccessCustomer: "2013226",
			PendingFailCustomer:   "2013230",
		}
	case strings.Contains(skuLower, "internet"):
		mapping = &TestSKUMapping{
			TestSKU:               "internet",
			SuccessCustomer:       "6391601001",
			FailCustomer:          "6391601002",
			PendingSuccessCustomer: "7391601001",
			PendingFailCustomer:   "7391601005",
		}
	case strings.Contains(skuLower, "bpjstk") && strings.Contains(skuLower, "pu"):
		mapping = &TestSKUMapping{
			TestSKU:               "bpjstkpu",
			SuccessCustomer:       "400000100001",
			FailCustomer:          "400000100002",
			PendingSuccessCustomer: "500000100001",
			PendingFailCustomer:   "500000100003",
		}
	case strings.Contains(skuLower, "bpjstk"):
		mapping = &TestSKUMapping{
			TestSKU:               "bpjstk",
			SuccessCustomer:       "8102051011270001",
			FailCustomer:          "8102051011270002",
			PendingSuccessCustomer: "9102051011270001",
			PendingFailCustomer:   "9102051011270003",
		}
	case strings.Contains(skuLower, "bpjs"):
		mapping = &TestSKUMapping{
			TestSKU:               "bpjs",
			SuccessCustomer:       "8801234560001",
			FailCustomer:          "8801234560002",
			PendingSuccessCustomer: "9801234560001",
			PendingFailCustomer:   "9801234560005",
		}
	case strings.Contains(skuLower, "multifinance"):
		mapping = &TestSKUMapping{
			TestSKU:               "multifinance",
			SuccessCustomer:       "6391601201",
			FailCustomer:          "6391601202",
			PendingSuccessCustomer: "7391601201",
			PendingFailCustomer:   "7391601205",
		}
	case strings.Contains(skuLower, "pbb") || strings.Contains(skuLower, "cimahi"):
		mapping = &TestSKUMapping{
			TestSKU:               "cimahi",
			SuccessCustomer:       "329801092375999991",
			FailCustomer:          "329801092375999992",
			PendingSuccessCustomer: "429801092375999991",
			PendingFailCustomer:   "429801092375999995",
		}
	case strings.Contains(skuLower, "pdl") || strings.Contains(skuLower, "pajak"):
		mapping = &TestSKUMapping{
			TestSKU:               "pdl",
			SuccessCustomer:       "3298010921",
			FailCustomer:          "3298010922",
			PendingSuccessCustomer: "4298010921",
			PendingFailCustomer:   "4298010923",
		}
	case strings.Contains(skuLower, "gas") || strings.Contains(skuLower, "pgas"):
		mapping = &TestSKUMapping{
			TestSKU:               "pgas",
			SuccessCustomer:       "0110014601",
			FailCustomer:          "0110014602",
			PendingSuccessCustomer: "1110014601",
			PendingFailCustomer:   "1110014605",
		}
	case strings.Contains(skuLower, "tv"):
		mapping = &TestSKUMapping{
			TestSKU:               "tv",
			SuccessCustomer:       "127246500101",
			FailCustomer:          "127246500102",
			PendingSuccessCustomer: "227246500101",
			PendingFailCustomer:   "227246500105",
		}
	case strings.Contains(skuLower, "emoney"):
		mapping = &TestSKUMapping{
			TestSKU:               "emoney",
			SuccessCustomer:       "082100000001",
			FailCustomer:          "082100000002",
			PendingSuccessCustomer: "082110000001",
			PendingFailCustomer:   "082110000003",
		}
	case strings.Contains(skuLower, "samsat"):
		mapping = &TestSKUMapping{
			TestSKU:               "samsat",
			SuccessCustomer:       "9658548523568701,0212502110170100",
			FailCustomer:          "9658548523568702,0212502110170100",
			PendingSuccessCustomer: "0658548523568701,0212502110170100",
			PendingFailCustomer:   "0658548523568705,0212502110170100",
		}
	case strings.Contains(skuLower, "hp") || strings.Contains(skuLower, "telkom"):
		mapping = &TestSKUMapping{
			TestSKU:               "hp",
			SuccessCustomer:       "081234554320",
			FailCustomer:          "081234554321",
			PendingSuccessCustomer: "081244554320",
			PendingFailCustomer:   "081244554324",
		}
	default:
		// Default to PLN if no match
		mapping = &TestSKUMapping{
			TestSKU:               "pln",
			SuccessCustomer:       "530000000001",
			FailCustomer:          "530000000003",
			PendingSuccessCustomer: "630000000001",
			PendingFailCustomer:   "630000000006",
		}
	}

	// Randomly choose one of the test customer numbers
	customers := []string{
		mapping.SuccessCustomer,
		mapping.FailCustomer,
		mapping.PendingSuccessCustomer,
		mapping.PendingFailCustomer,
	}

	testCustomerNo := customers[m.rnd.Intn(len(customers))]
	return mapping.TestSKU, testCustomerNo
}
