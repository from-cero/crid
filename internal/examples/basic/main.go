package main

import (
	"context"
	"fmt"
	"log"

	"github.com/from-cero/crid"
	"github.com/from-cero/crid/registry/memory"
)

func main() {
	ctx := context.Background()

	reg := memory.New()
	node, err := crid.New(reg)
	if err != nil {
		log.Fatalf("failed to create node: %v", err)
	}

	id, err := node.Generate(ctx)
	if err != nil {
		log.Fatalf("failed to generate ID: %v", err)
	}
	fmt.Println(id.String())
}
