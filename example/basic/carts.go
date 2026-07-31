package main

import (
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
)

const CartsCollectionName = "Carts" // from carts.schema.json/#/name

type Cart struct { // from carts.schema.json/#/fields
	data.DocumentModel
	UserID     string   `anansi:"user_id"`
	ProductIDs []string `anansi:"product_ids"`
	Quantity   int      `anansi:"quantity"`
}

type Carts struct { // this is just obvious
	*collection.ModelCollection[*Cart]
}
