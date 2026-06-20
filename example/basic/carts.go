package main

import (
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)

const CartsCollectionName = "Carts" // from carts.schema.json/#/name

type Cart struct { // from carts.schema.json/#/fields
	data.DocumentModel
	UserID  string   `doc:"user_id"`
	ProductIDs []string `doc:"product_ids"`
	Quantity int      `doc:"quantity"`
}

type Carts struct { // this is just obvious
	base.ModelCollection[Cart, *Cart]
}

