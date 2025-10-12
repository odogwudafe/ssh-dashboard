package main

import (
	"fmt"
	"os"

	"github.com/alpindale/ssh-dashboard/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	hosts, err := internal.ParseSSHConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing SSH config: %v\n", err)
		os.Exit(1)
	}

	if len(hosts) == 0 {
		fmt.Fprintf(os.Stderr, "No hosts found in SSH config\n")
		os.Exit(1)
	}

	p := tea.NewProgram(internal.InitialModel(hosts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
