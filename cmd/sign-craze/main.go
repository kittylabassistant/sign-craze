package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/kittylabassistant/sign-craze/internal/cli"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

func main() {
	if err := run(); err != nil {
		log.L().Error("dispatch failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	log.Init(os.Getenv("SIGNCRAZE_LOG_LEVEL"))
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return cli.Dispatch(ctx, os.Args[1:])
}
