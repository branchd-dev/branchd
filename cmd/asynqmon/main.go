package main

import (
	"log"
	"net/http"
	"os"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	h := asynqmon.New(asynqmon.Options{
		RootPath:     "/asynqmon",
		RedisConnOpt: asynq.RedisClientOpt{Addr: redisAddr},
	})

	port := os.Getenv("ASYNQMON_PORT")
	if port == "" {
		port = "8090"
	}

	log.Printf("Starting Asynqmon on :%s with Redis at %s", port, redisAddr)
	log.Fatal(http.ListenAndServe(":"+port, h))
}