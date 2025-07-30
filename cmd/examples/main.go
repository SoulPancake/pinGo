package main

import (
	"log"
	"os"

	"github.com/SoulPancake/pinGo/internal/examples"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "echo":
		examples.EchoServerExample()
	case "loadbalancer":
		examples.LoadBalancerExample()
	case "multi":
		examples.MultiServiceExample()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	log.Println(`pinGo Examples

Usage:
  examples echo         - Run echo server example
  examples loadbalancer - Run load balancer example  
  examples multi        - Run multi-service example`)
}