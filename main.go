package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Scrimzay/supplychain/db"
	"github.com/Scrimzay/supplychain/supplychain"
)

// Helper function to format Amount for display
func formatAmount(amount *supplychain.Amount) *supplychain.Amount {
	if amount == nil {
		return amount
	}
	// Assume USD with 2 decimal places for simplicity
	// value is in cents, so divide by 100 for dollars
	dollars := float64(amount.Value) / 100.0
	formatted := fmt.Sprintf("%.2f", dollars) // e.g., "1000.00"
	amount.DisplayValue = formatted
	return amount
}

// SupplyChainServer implements the SupplyChain service
type SupplyChainServer struct {
	supplychain.UnimplementedSupplyChainServer
	db *db.DatabaseStruct
}

func (s *SupplyChainServer) CreateItem(ctx context.Context, req *supplychain.CreateItemRequest) (*supplychain.CreateItemResponse, error) {
	if req.Name == "" || req.Quantity < 0 || req.UnitPrice.Value < 0 {
		return nil, status.Error(codes.InvalidArgument,  "Invalid item details")
	}

	item := &supplychain.Item{
		Id: uuid.New().String(),
		Name: req.Name,
		Description: req.Description,
		Quantity: req.Quantity,
		UnitPrice: formatAmount(req.UnitPrice),
		UpdatedAt: time.Now().Unix(),
	}

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO items (id, name, description, quantity, unit_price_value, unit_price_currency, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		item.Id, item.Name, item.Description, item.Quantity, item.UnitPrice.Value, item.UnitPrice.Currency, item.UpdatedAt)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to create item")
	}

	return &supplychain.CreateItemResponse{Item: item}, nil
}

func (s *SupplyChainServer) UpdateItem(ctx context.Context, req *supplychain.UpdateItemRequest) (*supplychain.UpdateItemResponse, error) {
	if req.Id == "" || req.Name == "" || req.Quantity < 0 || req.UnitPrice.Value < 0 {
		return nil, status.Error(codes.InvalidArgument, "Invalid item details")
	}

	item := &supplychain.Item{
		Id:          req.Id,
		Name:        req.Name,
		Description: req.Description,
		Quantity:    req.Quantity,
		UnitPrice:   formatAmount(req.UnitPrice),
		UpdatedAt:   time.Now().Unix(),
	}

	_, err := s.db.ExecContext(ctx,
		"UPDATE items SET name = ?, description = ?, quantity = ?, unit_price_value = ?, unit_price_currency = ?, updated_at = ? WHERE id = ?",
		item.Name, item.Description, item.Quantity, item.UnitPrice.Value, item.UnitPrice.Currency, item.UpdatedAt, item.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to update item")
	}

	return &supplychain.UpdateItemResponse{Item: item}, nil
}

func (s *SupplyChainServer) DeleteItem(ctx context.Context, req *supplychain.DeleteItemRequest) (*supplychain.DeleteItemResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "Item ID required")
	}

	result, err := s.db.ExecContext(ctx, "DELETE FROM items WHERE id = ?", req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to delete item")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, status.Error(codes.NotFound, "Item not found")
	}

	return &supplychain.DeleteItemResponse{Success: true}, nil
}

func (s *SupplyChainServer) CreateOrder(ctx context.Context, req *supplychain.CreateOrderRequest) (*supplychain.CreateOrderResponse, error) {
	if req.CustomerId == "" || len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Invalid order details")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to start tx")
	}
	defer tx.Rollback()

	var total int64
	for _, orderItem := range req.Items {
		var unitPrice int64
		err := tx.QueryRowContext(ctx, "SELECT unit_price_value FROM items WHERE id = ?", orderItem.ItemId).Scan(&unitPrice)
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "Item not found")
		}
		if err != nil {
			return nil, status.Error(codes.Internal, "Failed to check item")
		}
		total += unitPrice * int64(orderItem.Quantity)
	}

	order := &supplychain.Order{
		Id:         uuid.New().String(),
		CustomerId: req.CustomerId,
		Items:      req.Items,
		Total: formatAmount(&supplychain.Amount{
			Value:    total,
			Currency: "USD", // Assume USD for simplicity
		}),
		Status:    "PENDING",
		CreatedAt: time.Now().Unix(),
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO orders (id, customer_id, total_value, total_currency, status, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		order.Id, order.CustomerId, order.Total.Value, order.Total.Currency, order.Status, order.CreatedAt)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to create order")
	}

	for _, item := range order.Items {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO order_items (order_id, item_id, quantity) VALUES (?, ?, ?)",
			order.Id, item.ItemId, item.Quantity)
		if err != nil {
			return nil, status.Error(codes.Internal, "Failed to add order items")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "Failed to commit transaction")
	}

	return &supplychain.CreateOrderResponse{Order: order}, nil
}

func (s *SupplyChainServer) FulfillOrder(ctx context.Context, req *supplychain.FulfillOrderRequest) (*supplychain.FulfillOrderResponse, error) {
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "Order ID required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to start transaction")
	}
	defer tx.Rollback()

	var statusReport string
	err = tx.QueryRowContext(ctx, "SELECT status FROM orders WHERE id = ?", req.OrderId).Scan(&statusReport)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "Order not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to check order")
	}
	if statusReport != "PENDING" {
		return nil, status.Error(codes.FailedPrecondition, "Order cannot be fulfilled")
	}

	rows, err := tx.QueryContext(ctx, "SELECT item_id, quantity FROM order_items WHERE order_id = ?", req.OrderId)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to fetch order items")
	}
	defer rows.Close()

	for rows.Next() {
		var itemID string
		var quantity int32
		if err := rows.Scan(&itemID, &quantity); err != nil {
			return nil, status.Error(codes.Internal, "Failed to scan order items")
		}
		_, err = tx.ExecContext(ctx, "UPDATE items SET quantity = quantity - ? WHERE id = ? AND quantity >= ?", quantity, itemID, quantity)
		if err != nil {
			return nil, status.Error(codes.Internal, "Failed to update inventory")
		}
	}

	_, err = tx.ExecContext(ctx, "UPDATE orders SET status = 'FULFILLED' WHERE id = ?", req.OrderId)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to update order")
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "Failed to commit transaction")
	}

	order := &supplychain.Order{Id: req.OrderId, Status: "FULFILLED"}
	return &supplychain.FulfillOrderResponse{Order: order}, nil
}

func (s *SupplyChainServer) GetOrder(ctx context.Context, req *supplychain.GetOrderRequest) (*supplychain.GetOrderResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "Order ID required")
	}

	var order supplychain.Order
	var totalValue int64
	var totalCurrency, statusReport, customerID string
	var createdAt int64
	err := s.db.QueryRowContext(ctx,
		"SELECT id, customer_id, total_value, total_currency, status, created_at FROM orders WHERE id = ?",
		req.Id).Scan(&order.Id, &customerID, &totalValue, &totalCurrency, &statusReport, &createdAt)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "Order not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to fetch order")
	}

	rows, err := s.db.QueryContext(ctx, "SELECT item_id, quantity FROM order_items WHERE order_id = ?", req.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to fetch order items")
	}
	defer rows.Close()

	var items []*supplychain.OrderItem
	for rows.Next() {
		var itemID string
		var quantity int32
		if err := rows.Scan(&itemID, &quantity); err != nil {
			return nil, status.Error(codes.Internal, "Failed to scan order items")
		}
		items = append(items, &supplychain.OrderItem{ItemId: itemID, Quantity: quantity})
	}

	order.CustomerId = customerID
	order.Items = items
	order.Total = formatAmount(&supplychain.Amount{Value: totalValue, Currency: totalCurrency})
	order.Status = statusReport
	order.CreatedAt = createdAt

	return &supplychain.GetOrderResponse{Order: &order}, nil
}

func (s *SupplyChainServer) CreateShipment(ctx context.Context, req *supplychain.CreateShipmentRequest) (*supplychain.CreateShipmentResponse, error) {
	if req.OrderId == "" || req.TrackingNumber == "" {
		return nil, status.Error(codes.InvalidArgument, "Invalid shipment details")
	}

	var statusReport string
	err := s.db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id = ?", req.OrderId).Scan(&statusReport)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "Order not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to check order")
	}
	if statusReport != "FULFILLED" {
		return nil, status.Error(codes.FailedPrecondition, "Order must be fulfilled")
	}

	shipment := &supplychain.Shipment{
		Id:            uuid.New().String(),
		OrderId:       req.OrderId,
		Status:        "PENDING",
		TrackingNumber: req.TrackingNumber,
		UpdatedAt:     time.Now().Unix(),
	}

	_, err = s.db.ExecContext(ctx,
		"INSERT INTO shipments (id, order_id, status, tracking_number, updated_at) VALUES (?, ?, ?, ?, ?)",
		shipment.Id, shipment.OrderId, shipment.Status, shipment.TrackingNumber, shipment.UpdatedAt)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to create shipment")
	}

	return &supplychain.CreateShipmentResponse{Shipment: shipment}, nil
}

func (s *SupplyChainServer) UpdateShipment(ctx context.Context, req *supplychain.UpdateShipmentRequest) (*supplychain.UpdateShipmentResponse, error) {
	if req.Id == "" || req.Status == "" {
		return nil, status.Error(codes.InvalidArgument, "Invalid shipment details")
	}

	shipment := &supplychain.Shipment{
		Id:            req.Id,
		Status:        req.Status,
		TrackingNumber: req.TrackingNumber,
		UpdatedAt:     time.Now().Unix(),
	}

	_, err := s.db.ExecContext(ctx,
		"UPDATE shipments SET status = ?, tracking_number = ?, updated_at = ? WHERE id = ?",
		shipment.Status, shipment.TrackingNumber, shipment.UpdatedAt, shipment.Id)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to update shipment")
	}

	return &supplychain.UpdateShipmentResponse{Shipment: shipment}, nil
}

func (s *SupplyChainServer) ListItems(ctx context.Context, req *supplychain.ListItemsRequest) (*supplychain.ListItemsResponse, error) {
	if req.Page < 1 || req.PageSize < 1 {
		return nil, status.Error(codes.InvalidArgument, "Invalid pagination")
	}

	query := "SELECT id, name, description, quantity, unit_price_value, unit_price_currency, updated_at FROM items"
	args := []interface{}{}
	if req.NameFilter != "" {
		query += " WHERE name LIKE ?"
		args = append(args, "%"+req.NameFilter+"%")
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, (req.Page-1)*req.PageSize)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to list items")
	}
	defer rows.Close()

	var items []*supplychain.Item
	for rows.Next() {
		var item supplychain.Item
		var unitPriceValue int64
		var unitPriceCurrency string
		if err := rows.Scan(&item.Id, &item.Name, &item.Description, &item.Quantity, &unitPriceValue, &unitPriceCurrency, &item.UpdatedAt); err != nil {
			return nil, status.Error(codes.Internal, "Failed to scan items")
		}
		item.UnitPrice = formatAmount(&supplychain.Amount{Value: unitPriceValue, Currency: unitPriceCurrency})
		items = append(items, &item)
	}

	var total int32
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE name LIKE ?", "%"+req.NameFilter+"%").Scan(&total)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to count items")
	}

	return &supplychain.ListItemsResponse{Items: items, Total: total}, nil
}

func (s *SupplyChainServer) ListShipments(ctx context.Context, req *supplychain.ListShipmentsRequest) (*supplychain.ListShipmentsResponse, error) {
	if req.Page < 1 || req.PageSize < 1 {
		return nil, status.Error(codes.InvalidArgument, "Invalid pagination")
	}

	query := "SELECT id, order_id, status, tracking_number, updated_at FROM shipments"
	args := []interface{}{}
	if req.OrderId != "" {
		query += " WHERE order_id = ?"
		args = append(args, req.OrderId)
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, req.PageSize, (req.Page-1)*req.PageSize)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to list shipments")
	}
	defer rows.Close()

	var shipments []*supplychain.Shipment
	for rows.Next() {
		var shipment supplychain.Shipment
		if err := rows.Scan(&shipment.Id, &shipment.OrderId, &shipment.Status, &shipment.TrackingNumber, &shipment.UpdatedAt); err != nil {
			return nil, status.Error(codes.Internal, "Failed to scan shipments")
		}
		shipments = append(shipments, &shipment)
	}

	var total int32
	countQuery := "SELECT COUNT(*) FROM shipments"
	countArgs := []interface{}{}
	if req.OrderId != "" {
		countQuery += " WHERE order_id = ?"
		countArgs = append(countArgs, req.OrderId)
	}
	err = s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to count shipments")
	}

	return &supplychain.ListShipmentsResponse{Shipments: shipments, Total: total}, nil
}

// AuditLogs retrieves audit logs for a specific API key
func (s *SupplyChainServer) AuditLogs(ctx context.Context, req *supplychain.AuditLogsRequest) (*supplychain.AuditLogsResponse, error) {
	logs, err := s.db.GetAuditLogs(req.ApiKey, int(req.PageSize), int(req.Page-1)*int(req.PageSize))
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to fetch audit logs")
	}

	var protoLogs []*supplychain.AuditLog
	for _, log := range logs {
		protoLogs = append(protoLogs, &supplychain.AuditLog{
			Id:          log.ID,
			ApiKey:      log.APIKey,
			Method:      log.Method,
			RequestData: log.RequestData,
			Status:      log.Status,
			Timestamp:   log.Timestamp,
		})
	}

	var total int32
	err = s.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE api_key = ?", req.ApiKey).Scan(&total)
	if err != nil {
		return nil, status.Error(codes.Internal, "Failed to count audit logs")
	}

	return &supplychain.AuditLogsResponse{Logs: protoLogs, Total: total}, nil
}

//UnaryInterceptor for auth
func unaryInterceptor(db *db.DatabaseStruct) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// extract api key from metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "No metadata provided")
		}
		apiKeys := md.Get("api-key")
		if len(apiKeys) == 0 {
			return nil, status.Error(codes.Unauthenticated, "API key required")
		}
		apiKey := apiKeys[0]

		// validate api key
		role, err := db.ValidateAPIKey(apiKey)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		// Define allowed methods per role
		allowedMethods := map[string][]string{
			"customer": {
				"/supplychain.SupplyChain/CreateOrder",
				"/supplychain.SupplyChain/ListItems",
				"/supplychain.SupplyChain/GetOrder",
			},
			"admin": {
				"/supplychain.SupplyChain/CreateItem",
				"/supplychain.SupplyChain/UpdateItem",
				"/supplychain.SupplyChain/DeleteItem",
				"/supplychain.SupplyChain/CreateOrder",
				"/supplychain.SupplyChain/FulfillOrder",
				"/supplychain.SupplyChain/GetOrder",
				"/supplychain.SupplyChain/CreateShipment",
				"/supplychain.SupplyChain/UpdateShipment",
				"/supplychain.SupplyChain/ListItems",
				"/supplychain.SupplyChain/ListShipments",
				"/supplychain.SupplyChain/AuditLogs",
			},
		}

		allowed := false
		// check if the method is allowed for the role
		for _, method := range allowedMethods[role] {
			if method == info.FullMethod {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, status.Error(codes.PermissionDenied, "Method not allowed for role")
		}

		// serialize request to json
		requestData, err := json.Marshal(req)
		if err != nil {
			log.Printf("Failed to serialize request: %v", err)
			requestData = []byte("{}")
		}

		// call the handler
		resp, err := handler(ctx, req)

		// log the request
		logStatus := "success"
		if err != nil {
			logStatus = status.Code(err).String()
		}
		_, dbErr := db.ExecContext(ctx,
		"INSERT INTO audit_logs (api_key, method, request_data, status, timestamp) VALUES (?, ?, ?, ?, ?)",
		apiKey, info.FullMethod, string(requestData), logStatus, time.Now().Unix())
		if dbErr != nil {
			log.Printf("Failed to save audit log: %v", err)
		}

		return resp, err
	}
}

func main() {
	db, err := db.InitDB("supplychain.db")
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer db.Close()

	lis, err := net.Listen("tcp", ":8089")
	if err != nil {
		log.Fatalf("Could not listen: %v", err)
	}

	// create a grpc server with interceptor
	server := grpc.NewServer(
		grpc.UnaryInterceptor(unaryInterceptor(db)),
	)
	service := &SupplyChainServer{db: db}

	// register service
	supplychain.RegisterSupplyChainServer(server, service)

	log.Println("Starting gRPC server on :8089")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}