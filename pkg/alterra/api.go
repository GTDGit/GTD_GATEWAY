package alterra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// GetProducts retrieves the product list
func (c *Client) GetProducts(ctx context.Context, page, perPage int) (*ProductListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}

	path := fmt.Sprintf("/api/v5/product?page=%d&per_page=%d", page, perPage)

	var resp ProductListResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetAllProducts retrieves all products (paginated)
func (c *Client) GetAllProducts(ctx context.Context) ([]Product, error) {
	var allProducts []Product
	page := 1
	perPage := 100
	const maxPages = 1000 // Guard against infinite loop

	for page <= maxPages {
		resp, err := c.GetProducts(ctx, page, perPage)
		if err != nil {
			return nil, err
		}

		allProducts = append(allProducts, resp.Data...)

		// Exit if no more pages or empty response
		if resp.TotalPages == 0 || page >= resp.TotalPages || len(resp.Data) == 0 {
			break
		}
		page++
	}

	return allProducts, nil
}

// GetBalance retrieves account balance
func (c *Client) GetBalance(ctx context.Context) (*BalanceResponse, error) {
	var resp BalanceResponse
	if err := c.doRequest(ctx, http.MethodGet, "/api/v5/balance", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Purchase performs a prepaid purchase transaction
func (c *Client) Purchase(ctx context.Context, customerID string, productID int, orderID string, data json.RawMessage) (*TransactionResponse, error) {
	if data == nil {
		data = json.RawMessage("{}")
	}

	req := PurchaseRequest{
		CustomerID: customerID,
		ProductID:  productID,
		OrderID:    orderID,
		Data:       data,
	}

	var resp TransactionResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v5/transaction/purchase", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Inquiry performs a postpaid inquiry
func (c *Client) Inquiry(ctx context.Context, customerID string, productID int, orderID string, data json.RawMessage) (*TransactionResponse, error) {
	// Build inquiry data: product_id is required inside data
	inquiryData := map[string]any{
		"product_id": productID,
	}
	// Merge any additional data fields
	if len(data) > 0 && string(data) != "{}" {
		var extra map[string]any
		if err := json.Unmarshal(data, &extra); err == nil {
			for k, v := range extra {
				inquiryData[k] = v
			}
		}
	}
	dataJSON, _ := json.Marshal(inquiryData)

	req := InquiryRequest{
		CustomerID:  customerID,
		InquiryType: "Customer_information",
		Data:        dataJSON,
	}

	var resp TransactionResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v5/transaction/inquiry", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Payment performs a postpaid payment (uses /purchase endpoint per Alterra docs)
func (c *Client) Payment(ctx context.Context, customerID string, productID int, orderID string, data json.RawMessage) (*TransactionResponse, error) {
	if data == nil {
		data = json.RawMessage("{}")
	}

	req := PaymentRequest{
		CustomerID: customerID,
		ProductID:  productID,
		OrderID:    orderID,
		Data:       data,
	}

	var resp TransactionResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v5/transaction/purchase", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransactionByOrderID retrieves transaction by order ID
func (c *Client) GetTransactionByOrderID(ctx context.Context, orderID string) (*TransactionDetailResponse, error) {
	path := fmt.Sprintf("/api/v5/transaction/order_id/%s", url.PathEscape(orderID))

	var resp TransactionDetailResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransactionByID retrieves transaction by Alterra transaction ID
func (c *Client) GetTransactionByID(ctx context.Context, transactionID int) (*TransactionDetailResponse, error) {
	path := fmt.Sprintf("/api/v5/transaction/%d", transactionID)

	var resp TransactionDetailResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
