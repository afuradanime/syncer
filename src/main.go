package main

import (
	"flag"
	"fmt"
	"os"

	"syncer/src/config"
	"syncer/src/sync"
)

const DEFAULT_EXECUTION_MODE = "full"
const DEFAULT_CONFIG_PATH = "config.ini"

func main() {

	executionMode := flag.String("mode", DEFAULT_EXECUTION_MODE, "Mode to run the program as")
	configPath := flag.String("config", DEFAULT_CONFIG_PATH, "Path to the config file")

	flag.Parse()

	fmt.Printf("Starting syncer...\n")
	fmt.Printf("Execution mode: %s\n", *executionMode)
	fmt.Printf("Config path: %s\n", *configPath)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %s", err)
		return
	}

	syncer := sync.Syncer{
		Config: cfg,
	}

	switch *executionMode {
	case "full":
		if err := syncer.RunFullSync(); err != nil {
			fmt.Printf("Full sync failed: %v\n", err)
			os.Exit(1)
		}
	case "partial":
		if err := syncer.RunPartialSync(); err != nil {
			fmt.Printf("Partial sync failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown execution mode: %s\n", *executionMode)
	}
}
