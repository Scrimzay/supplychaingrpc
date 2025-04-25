just some grpc prac:

tutorial:

theres already a customer and 2 admin users pre-defined 

1. ./supplychaincli -apikey admin-key-456 -createitem -name "Laptop" -description "High Performance Laptop" -quantity 10 -price 1000.00 -currency USD

2. ./supplychaincli -apikey customer-key-123 -createorder -customer LAPTOPSTORE001 -item {id of item, stored in db or check cli output after creating item} -quantity 1

3. ./supplychaincli -apikey admin-key-456 -fulfillorder -order {id of order, stored in db or check cli output after creating an order}

4. ./supplychaincli -apikey admin-key-456 -createshipment  -order {same thing as step 3} -tracking LAPTOPSTORE001TRACKING001

5. ./supplychaincli -apikey admin-key-456 -updateshipment -id {id of shipment, stored in db or check cli output after creating a shipment} -status FULFILLED -tracking LAPTOPSTORE001TRACKING001

6. ./supplychaincli -apikey admin-key-456 -updateitem -id {id of item to update, stored in db} -name "Laptop" -description "High Performance Laptop" -quantity 10 -price 1100.00 -currency USD

7. ./supplychaincli -apikey adminkey-789 -audit -auditkey admin-key-456

thats basically how it works
