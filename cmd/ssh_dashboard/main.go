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

func validateInterval(seconds int) time.Duration {
	if seconds < 1 || seconds > 3600 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func main() {
	flag.Usage = func() {
		// HACK: make it look like python's argparse
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -n, --interval int    Update interval in seconds (default: 5, or SSH_DASHBOARD_INTERVAL env var)\n")
		fmt.Fprintf(os.Stderr, "  -h, --help            Show this help message\n")
	}

	var updateIntervalVal int
	flag.IntVar(&updateIntervalVal, "n", 0, "")
	flag.IntVar(&updateIntervalVal, "interval", 0, "")
	flag.Parse()
	updateInterval := &updateIntervalVal

	interval := 5 * time.Second

	if *updateInterval > 0 {
		if validated := validateInterval(*updateInterval); validated > 0 {
			interval = validated
		}
	} else if envInterval := os.Getenv("SSH_DASHBOARD_INTERVAL"); envInterval != "" {
		if seconds, err := strconv.Atoi(envInterval); err == nil {
			if validated := validateInterval(seconds); validated > 0 {
				interval = validated
			}
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
