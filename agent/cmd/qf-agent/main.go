package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/qf/qf/agent/internal/agent"
	"github.com/qf/qf/agent/internal/loader"
)

func main() {
	iface := "eth0"
	if len(os.Args) > 1 {
		iface = os.Args[1]
	}

	l, err := loader.Load(iface)
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	defer l.Close()

	logger := log.New(os.Stderr, "[qf] ", log.LstdFlags)
	ag := agent.New(l, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("qf attached on %s", iface)
	if err := ag.Start(ctx); err != nil {
		log.Fatalf("agent: %v", err)
	}
	log.Println("detaching...")
}
