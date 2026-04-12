package main

import (
"flag"
"fmt"
"os"

"github.com/cloud-mirror/state-sync-engine/internal/config"
)

func main() {
configPath := flag.String("config", "config.yaml", "Path to configuration file (YAML or JSON)")
flag.Parse()

_, err := config.Load(*configPath)
if err != nil {
fmt.Fprintf(os.Stderr, "startup failed: %v\n", err)
os.Exit(1)
}

fmt.Println("State Sync Engine - Cloud Mirror")
fmt.Println("Configuration loaded successfully")
fmt.Println("Starting...")
}
