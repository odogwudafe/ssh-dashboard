package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/alpindale/ssh-dashboard/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	updateInterval := flag.Int("interval", 0, "Update interval in seconds (default: 10, or SSH_DASHBOARD_INTERVAL env var)")
	flag.Parse()

	// Determine update interval: CLI flag > env var > default (10s)
	interval := 10 * time.Second
	if *updateInterval > 0 {
		interval = time.Duration(*updateInterval) * time.Second
	} else if envInterval := os.Getenv("SSH_DASHBOARD_INTERVAL"); envInterval != "" {
		if seconds, err := strconv.Atoi(envInterval); err == nil && seconds > 0 {
			interval = time.Duration(seconds) * time.Second
		}
	}

	hosts, err := internal.ParseSSHConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SSH config: %v\n", err)
		os.Exit(1)
	}

	if len(hosts) == 0 {
		fmt.Fprintf(os.Stderr, "No hosts found in SSH config\n")
		os.Exit(1)
	}

	p := tea.NewProgram(internal.InitialModel(hosts, interval), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
