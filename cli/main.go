package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/Scrimzay/supplychain/supplychain"
)

func main() {
	// Define global flags
	connectAddr := flag.String("connect", "localhost:8089", "gRPC server address")
	apiKey := flag.String("apikey", "", "API key for authentication")

	// Define command flags
	createItem := flag.Bool("createitem", false, "Create a new item")
	updateItem := flag.Bool("updateitem", false, "Update an existing item")
	deleteItem := flag.Bool("deleteitem", false, "Delete an item")
	createOrder := flag.Bool("createorder", false, "Create a new order")
	fulfillOrder := flag.Bool("fulfillorder", false, "Fulfill an order")
	getOrder := flag.Bool("getorder", false, "Get order details")
	createShipment := flag.Bool("createshipment", false, "Create a shipment")
	updateShipment := flag.Bool("updateshipment", false, "Update a shipment")
	listItems := flag.Bool("listitems", false, "List items")
	listShipments := flag.Bool("listshipments", false, "List shipments")
	audit := flag.Bool("audit", false, "View audit logs for an API key")

	// Define argument flags
	name := flag.String("name", "", "Item name")
	description := flag.String("description", "", "Item description")
	quantity := flag.Int("quantity", 0, "Item or order quantity")
	price := flag.Float64("price", 0, "Item price in dollars (e.g., 1000.00)")
	currency := flag.String("currency", "USD", "Currency (e.g., USD)")
	id := flag.String("id", "", "Item or shipment ID")
	customer := flag.String("customer", "", "Customer ID for order")
	itemID := flag.String("item", "", "Item ID for order")
	orderID := flag.String("order", "", "Order ID")
	trackingNumber := flag.String("tracking", "", "Shipment tracking number")
	status := flag.String("status", "", "Shipment status")
	nameFilter := flag.String("namefilter", "", "Filter for listing items")
	auditKey := flag.String("auditkey", "", "API key to audit")
	page := flag.Int("page", 1, "Page number for listing or audit")
	pageSize := flag.Int("pagesize", 10, "Page size for listing or audit")

	flag.Parse()

	if *apiKey == "" {
		log.Fatal("API key is required (-apiKey)")
	}

	// Connect to gRPC server
	conn, err := grpc.Dial(*connectAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *connectAddr, err)
	}
	defer conn.Close()

	client := supplychain.NewSupplyChainClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "api-key", *apiKey)

	// handle commands
	switch {
	case *createItem:
		if *name == "" || *quantity <= 0 || *price <= 0 {
			log.Fatal("Required flags for -createitem: -name, -quantity, -price")
		}
		req := &supplychain.CreateItemRequest{
			Name: *name,
			Description: *description,
			Quantity: int32(*quantity),
			UnitPrice: &supplychain.Amount{
				Value: int64(*price * 100),
				Currency: *currency,
			},
		}
		resp, err := client.CreateItem(ctx, req)
		if err != nil {
			log.Fatalf("Failed to create item: %v", err)
		}
		fmt.Printf("Created item: %s (ID: %s), Quantity: %d, Unit Price: %s %s\n",
			resp.Item.Name, resp.Item.Id, resp.Item.Quantity,
			resp.Item.UnitPrice.DisplayValue, resp.Item.UnitPrice.Currency)

	case *updateItem:
		if *id == "" || *name == "" || *quantity < 0 || *price <= 0 {
			log.Fatal("Required flags for -updateitem: -id, -name, -quantity, -price")
		}
		req := &supplychain.UpdateItemRequest{
			Id:          *id,
			Name:        *name,
			Description: *description,
			Quantity:    int32(*quantity),
			UnitPrice: &supplychain.Amount{
				Value:    int64(*price * 100),
				Currency: *currency,
			},
		}
		resp, err := client.UpdateItem(ctx, req)
		if err != nil {
			log.Fatalf("Failed to update item: %v", err)
		}
		fmt.Printf("Updated item: %s (ID: %s), Quantity: %d, Unit Price: %s %s\n",
			resp.Item.Name, resp.Item.Id, resp.Item.Quantity,
			resp.Item.UnitPrice.DisplayValue, resp.Item.UnitPrice.Currency)

	case *deleteItem:
		if *id == "" {
			log.Fatal("Required flag for -deleteitem: -id")
		}
		req := &supplychain.DeleteItemRequest{Id: *id}
		resp, err := client.DeleteItem(ctx, req)
		if err != nil {
			log.Fatalf("Failed to delete item: %v", err)
		}
		fmt.Printf("Deleted item: Success=%v\n", resp.Success)

	case *createOrder:
		if *customer == "" || *itemID == "" || *quantity <= 0 {
			log.Fatal("Required flags for -createorder: -customer, -item, -quantity")
		}
		req := &supplychain.CreateOrderRequest{
			CustomerId: *customer,
			Items: []*supplychain.OrderItem{
				{ItemId: *itemID, Quantity: int32(*quantity)},
			},
		}
		resp, err := client.CreateOrder(ctx, req)
		if err != nil {
			log.Fatalf("Failed to create order: %v", err)
		}
		fmt.Printf("Created order: %s, Total: %s %s\n",
			resp.Order.Id, resp.Order.Total.DisplayValue, resp.Order.Total.Currency)

	case *fulfillOrder:
		if *orderID == "" {
			log.Fatal("Required flag for -fulfillorder: -order")
		}
		req := &supplychain.FulfillOrderRequest{OrderId: *orderID}
		resp, err := client.FulfillOrder(ctx, req)
		if err != nil {
			log.Fatalf("Failed to fulfill order: %v", err)
		}
		fmt.Printf("Fulfilled order: %s, Status: %s\n", resp.Order.Id, resp.Order.Status)

	case *getOrder:
		if *orderID == "" {
			log.Fatal("Required flag for -getorder: -order")
		}
		req := &supplychain.GetOrderRequest{Id: *orderID}
		resp, err := client.GetOrder(ctx, req)
		if err != nil {
			log.Fatalf("Failed to get order: %v", err)
		}
		fmt.Printf("Order: %s, Customer: %s, Total: %s %s, Status: %s\n",
			resp.Order.Id, resp.Order.CustomerId,
			resp.Order.Total.DisplayValue, resp.Order.Total.Currency, resp.Order.Status)

	case *createShipment:
		if *orderID == "" || *trackingNumber == "" {
			log.Fatal("Required flags for -createshipment: -order, -tracking")
		}
		req := &supplychain.CreateShipmentRequest{
			OrderId:        *orderID,
			TrackingNumber: *trackingNumber,
		}
		resp, err := client.CreateShipment(ctx, req)
		if err != nil {
			log.Fatalf("Failed to create shipment: %v", err)
		}
		fmt.Printf("Created shipment: %s, Order: %s, Tracking: %s, Status: %s\n",
			resp.Shipment.Id, resp.Shipment.OrderId, resp.Shipment.TrackingNumber, resp.Shipment.Status)
	
	case *updateShipment:
		if *id == "" || *status == "" || *trackingNumber == "" {
			log.Fatal("Required flags for -updateshipment: -id, -status -tracking")
		}
		req := &supplychain.UpdateShipmentRequest{
			Id:             *id,
			Status:         *status,
			TrackingNumber: *trackingNumber,
		}
		resp, err := client.UpdateShipment(ctx, req)
		if err != nil {
			log.Fatalf("Failed to update shipment: %v", err)
		}
		fmt.Printf("Updated shipment: %s, Status: %s, Tracking: %s\n",
			resp.Shipment.Id, resp.Shipment.Status, resp.Shipment.TrackingNumber)
	
	case *listItems:
		req := &supplychain.ListItemsRequest{
			NameFilter: *nameFilter,
			Page:       int32(*page),
			PageSize:   int32(*pageSize),
		}
		resp, err := client.ListItems(ctx, req)
		if err != nil {
			log.Fatalf("Failed to list items: %v", err)
		}
		fmt.Printf("Listed %d items (Total: %d):\n", len(resp.Items), resp.Total)
		for _, item := range resp.Items {
			fmt.Printf("  Item: %s (ID: %s), Quantity: %d, Unit Price: %s %s\n",
				item.Name, item.Id, item.Quantity,
				item.UnitPrice.DisplayValue, item.UnitPrice.Currency)
		}
	
	case *listShipments:
		req := &supplychain.ListShipmentsRequest{
			OrderId:  *orderID,
			Page:     int32(*page),
			PageSize: int32(*pageSize),
		}
		resp, err := client.ListShipments(ctx, req)
		if err != nil {
			log.Fatalf("Failed to list shipments: %v", err)
		}
		fmt.Printf("Listed %d shipments (Total: %d):\n", len(resp.Shipments), resp.Total)
		for _, shipment := range resp.Shipments {
			fmt.Printf("  Shipment: %s, Order: %s, Tracking: %s, Status: %s\n",
				shipment.Id, shipment.OrderId, shipment.TrackingNumber, shipment.Status)
		}

	case *audit:
		if *auditKey == "" {
			log.Fatal("Required flag for -audit: -auditkey")
		}
		req := &supplychain.AuditLogsRequest{
			ApiKey:   *auditKey,
			Page:     int32(*page),
			PageSize: int32(*pageSize),
		}
		resp, err := client.AuditLogs(ctx, req)
		if err != nil {
			log.Fatalf("Failed to get audit logs: %v", err)
		}
		fmt.Printf("Audit logs for API key %s (Page %d, Size %d, Total %d):\n", *auditKey, *page, *pageSize, resp.Total)
		for _, log := range resp.Logs {
			t := time.Unix(log.Timestamp, 0).Format(time.RFC3339)
			fmt.Printf("  ID: %d, Method: %s, Status: %s, Time: %s\n", log.Id, log.Method, log.Status, t)
			fmt.Printf("    Request: %s\n", log.RequestData)
		}
	
	default:
		log.Fatal("No command specified (e.g., -createitem, -createorder)")
	}
}