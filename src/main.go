package main

import (
	"flag"
	"fmt"

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
		syncer.RunFullSync()
	case "partial":
		syncer.RunPartialSync()
	default:
		fmt.Printf("Unknown execution mode: %s\n", *executionMode)
	}
}
