package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/greenmochi/kabedon-kokoro/gateway"
	"github.com/greenmochi/kabedon-kokoro/kokoro"
	"github.com/greenmochi/kabedon-kokoro/logger"
	"github.com/greenmochi/kabedon-kokoro/process"
)

func main() {
	// Setup logger
	defer logger.Close()

	var helpUsage bool
	var gatewayPort int
	var kokoroPort int
	var nyaaPort int
	flag.BoolVar(&helpUsage, "help", false, "Prints help text")
	flag.IntVar(&gatewayPort, "gateway-port", 9990, "Port to serve the gateway server")
	flag.IntVar(&kokoroPort, "kokoro-port", 9991, "Port to serve the kokoro server")
	flag.IntVar(&nyaaPort, "nyaa-port", 9995, "Nyaa grpc server port")
	flag.Parse()
	flag.Visit(func(fn *flag.Flag) {
		if fn.Name == "help" {
			fmt.Print(helpText)
			os.Exit(1)
		}
	})

	services := map[string]process.Service{
		"nyaa": process.Service{
			Name:   "kabedon-nyaa",
			Binary: "kabedon-nyaa.exe",
			Dir:    "./kabedon-nyaa",
			Args: []string{
				fmt.Sprintf("--port=%d", nyaaPort),
			},
			Port:     nyaaPort,
			Endpoint: fmt.Sprintf("http://localhost:%d", nyaaPort),
			FullPath: "./kabedon-nyaa/kabedon-nyaa.exe",
		},
	}

	shutdown := make(chan bool)
	exit := make(chan bool)
	release := make(chan bool)

	// Run all gRPC services
	for _, service := range services {
		go func(service process.Service) {
			cmd, err := process.Start(service.Binary, service.Dir, service.Args)
			if err != nil {
				logger.Errorf("unable to start %s: %s", service.Name, err)
				logger.Errorf("%+v\n", service)
			}
			logger.Infof("running %s on port=%d", service.FullPath, service.Port)

			// Wait for release signal when kabedon-kokoro finishes
			<-release

			if err := cmd.Process.Kill(); err != nil {
				logger.Fatalf("unable to kill %s: %s", service.Binary, err)
			}
			logger.Infof("killed %s", service.Binary)
			logger.Infof("%s exited", service.Binary)

			exit <- true
		}(service)
	}

	// endpoints := map[string]string{
	// 	"nyaa": fmt.Sprintf("localhost:%d", nyaaPort),
	// }

	// Load and run all gateway handlers on a port
	go func() {
		logger.Infof("running gateway server on :%d", gatewayPort)
		if err := gateway.Run(gatewayPort, services); err != nil {
			logger.Fatal(err)
		}
	}()

	// Run secondary server
	go func() {
		logger.Infof("running kokoro server on :%d", kokoroPort)
		if err := kokoro.Run(kokoroPort, services, shutdown); err != nil {
			logger.Fatal(err)
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Graceful shutdown
	logger.Infof("graceful shutdown loop started")
	for {
		select {
		case <-interrupt:
			logger.Info("interrupt signal received")
			release <- true
		case <-shutdown:
			logger.Info("shutdown signal received")
			release <- true
		case <-exit:
			logger.Info("exit signal received. Program exited.")
			os.Exit(1)
			return
		}
	}
}

const helpText = `Usage: kabedon-kokoro [options]

kabedon-kokoro converts REST to gRPC calls, and provides a secondary server
to log information and control the gRPC services.

Options:
  --help              Prints program help text
  
  --gateway-port=PORT Run gateway on PORT
  --kokoro-port=PORT  Run secondary server on PORT
  
  --nyaa-port=PORT    Run kabedon-nyaa service on PORT
`
