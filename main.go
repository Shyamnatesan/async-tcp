package main

import (
	"os"
	"os/signal"
	"syscall"
)



func main() {
	port := 8080
	host := "127.0.0.1"
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM) // Catch Ctrl + C (SIGINT)
	server := NewAsyncServer(port, host)

	go func ()  {
		<-sigChan
		server.shutdown()
	}()
	server.serve()

}