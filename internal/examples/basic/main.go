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

	parser, err := crid.NewParser()
	if err != nil {
		fmt.Printf("failed to create parser: %v", err)
		return
	}

	for i := 0; i < 5; i++ {
		id, err := node.Generate(ctx)
		if err != nil {
			log.Fatalf("failed to generate ID: %v", err)
		}
		fmt.Printf("%s -> %s\n", id.String(), parser.Parse(id))
	}
}
