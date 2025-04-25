package main

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/Scrimzay/supplychain/supplychain"
)

func main() {
	conn, err := grpc.Dial("localhost:8089", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := supplychain.NewSupplyChainClient(conn)

	// create context with api key
	ctx := metadata.AppendToOutgoingContext(context.Background(), "api-key", "admin-key-456")

	// Create an item
	createItemReq := &supplychain.CreateItemRequest{
		Name:        "Laptop",
		Description: "High-performance laptop",
		Quantity:    10,
		UnitPrice: &supplychain.Amount{
			Value:    100000, // $1000.00
			Currency: "USD",
		},
	}
	itemResp, err := client.CreateItem(ctx, createItemReq)
	if err != nil {
		log.Fatalf("Failed to create item: %v", err)
	}
	log.Printf("Created item: %+v\n", itemResp.Item)

	// Create an order
	createOrderReq := &supplychain.CreateOrderRequest{
		CustomerId: "CUST001",
		Items: []*supplychain.OrderItem{
			{ItemId: itemResp.Item.Id, Quantity: 1},
		},
	}
	orderResp, err := client.CreateOrder(ctx, createOrderReq)
	if err != nil {
		log.Fatalf("Failed to create order: %v", err)
	}
	log.Printf("Created order: %+v\n", orderResp.Order)

	// Fulfill the order
	fulfillReq := &supplychain.FulfillOrderRequest{OrderId: orderResp.Order.Id}
	fulfillResp, err := client.FulfillOrder(ctx, fulfillReq)
	if err != nil {
		log.Fatalf("Failed to fulfill order: %v", err)
	}
	log.Printf("Fulfilled order: %+v\n", fulfillResp.Order)

	// Create a shipment
	createShipmentReq := &supplychain.CreateShipmentRequest{
		OrderId:        orderResp.Order.Id,
		TrackingNumber: "TRK123456",
	}
	shipmentResp, err := client.CreateShipment(ctx, createShipmentReq)
	if err != nil {
		log.Fatalf("Failed to create shipment: %v", err)
	}
	log.Printf("Created shipment: %+v\n", shipmentResp.Shipment)

	// Restock item using UpdateItem
	updateItemReq := &supplychain.UpdateItemRequest{
		Id:          itemResp.Item.Id, // Use the item ID from CreateItem
		Name:        "Laptop",
		Description: "High-performance laptop",
		Quantity:    10,
		UnitPrice: &supplychain.Amount{
			Value:    100000, // $1000.00 in cents
			Currency: "USD",
		},
	}
	updateItemResp, err := client.UpdateItem(ctx, updateItemReq)
	if err != nil {
		log.Fatalf("Failed to update item: %v", err)
	}
	log.Printf("Updated item: %s, Quantity: %d, Unit Price: %s %s",
		updateItemResp.Item.Name,
		updateItemResp.Item.Quantity,
		updateItemResp.Item.UnitPrice.DisplayValue,
		updateItemResp.Item.UnitPrice.Currency,
	)

	// List items
	listItemsReq := &supplychain.ListItemsRequest{NameFilter: "Laptop", Page: 1, PageSize: 10}
	listItemsResp, err := client.ListItems(ctx, listItemsReq)
	if err != nil {
		log.Fatalf("Failed to list items: %v", err)
	}
	log.Printf("Listed %d items:\n", len(listItemsResp.Items))
	for _, item := range listItemsResp.Items {
		log.Printf("%+v\n", item)
	}
}