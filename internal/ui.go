package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Screen int

const (
	ScreenHostList Screen = iota
	ScreenConnecting
	ScreenDashboard
	ScreenError
)

type Model struct {
	screen       Screen
	hosts        []SSHHost
	selectedHost *SSHHost
	list         list.Model
	client       *SSHClient
	sysInfo      *SystemInfo
	updateCount  int
	lastUpdate   time.Time
	err          error
	width        int
	height       int
}

type TickMsg time.Time

type SystemInfoMsg struct {
	info *SystemInfo
	err  error
}

type ConnectedMsg struct {
	client *SSHClient
	err    error
}

type hostItem SSHHost

func (h hostItem) FilterValue() string { return h.Name }
func (h hostItem) Title() string       { return h.Name }
func (h hostItem) Description() string {
	if h.Hostname != "" {
		return fmt.Sprintf("%s@%s:%s", h.User, censorHostname(h.Hostname), h.Port)
	}
	return ""
}

func censorHostname(hostname string) string {
	if hostname == "" {
		return ""
	}

	// for IP addresses (contains dots)
	if strings.Contains(hostname, ".") {
		parts := strings.Split(hostname, ".")
		if len(parts) >= 4 {
			// IPv4: show first octet, censor middle, show last 2 chars of last octet
			lastOctet := parts[len(parts)-1]
			lastPart := lastOctet
			if len(lastOctet) > 2 {
				lastPart = lastOctet[len(lastOctet)-2:]
			}
			return fmt.Sprintf("%s.***.***%s", parts[0], lastPart)
		}
	}

	// for hostnames: show first 3 chars and last 3 chars
	if len(hostname) <= 8 {
		if len(hostname) <= 3 {
			return hostname
		}
		return hostname[:2] + strings.Repeat("*", len(hostname)-2)
	}

	// longer hostname: show first 3 and last 3 with asterisks in between
	return hostname[:3] + strings.Repeat("*", 5) + hostname[len(hostname)-3:]
}

func InitialModel(hosts []SSHHost) Model {
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem(h)
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select an SSH Host to Monitor"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return Model{
		screen: ScreenHostList,
		hosts:  hosts,
		list:   l,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.client != nil {
				m.client.Close()
			}
			return m, tea.Quit
		case "enter":
			if m.screen == ScreenHostList {
				if item, ok := m.list.SelectedItem().(hostItem); ok {
					host := SSHHost(item)
					m.selectedHost = &host
					m.screen = ScreenConnecting
					return m, m.connectToHost()
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)

	case ConnectedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.screen = ScreenError
			return m, nil
		}
		m.client = msg.client
		m.screen = ScreenDashboard
		return m, tea.Batch(m.gatherSysInfo(), m.tick())

	case SystemInfoMsg:
		if msg.err != nil {
			m.err = msg.err
			m.screen = ScreenError
			return m, nil
		}
		m.sysInfo = msg.info
		m.updateCount++
		m.lastUpdate = time.Now()

	case TickMsg:
		// update every 10 seconds
		return m, tea.Batch(m.gatherSysInfo(), m.tick())
	}

	if m.screen == ScreenHostList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	switch m.screen {
	case ScreenHostList:
		return m.list.View()

	case ScreenConnecting:
		return renderConnecting(m.selectedHost.Name)

	case ScreenDashboard:
		if m.sysInfo != nil {
			return renderDashboard(m.selectedHost.Name, m.sysInfo, m.updateCount, m.lastUpdate, m.width, m.height)
		}
		return "Loading system information..."

	case ScreenError:
		return renderError(m.err)
	}

	return ""
}

func (m Model) connectToHost() tea.Cmd {
	return func() tea.Msg {
		client, err := NewSSHClient(*m.selectedHost)
		return ConnectedMsg{client: client, err: err}
	}
}

func (m Model) gatherSysInfo() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return SystemInfoMsg{err: fmt.Errorf("no active SSH connection")}
		}
		info, err := GatherSystemInfo(m.client)
		return SystemInfoMsg{info: info, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("63"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196")).
			Padding(1, 2)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)
)

func renderProgressBar(percent float64, width int, color lipgloss.Color) string {
	if percent > 100 {
		percent = 100
	}
	if percent < 0 {
		percent = 0
	}

	filled := int(float64(width) * percent / 100.0)
	empty := width - filled

	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))

	return bar
}

func renderConnecting(hostName string) string {
	return boxStyle.Render(fmt.Sprintf("Connecting to %s...\n\nPlease wait...", hostName))
}

func renderError(err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %v\n\nPress 'q' to quit", err))
}

func renderDashboard(hostName string, info *SystemInfo, updateCount int, lastUpdate time.Time, width, height int) string {
	var b strings.Builder

	title := fmt.Sprintf("  System Dashboard - %s  ", hostName)
	subtitle := fmt.Sprintf("Last Updated: %s | Updates: %d | Press 'q' to quit",
		lastUpdate.Format("15:04:05"), updateCount)

	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(subtitle))
	b.WriteString("\n\n")

	if len(info.GPUs) > 0 {
		b.WriteString(renderGPUSection(info.GPUs))
		b.WriteString("\n")
	}

	b.WriteString(renderRAMAndDiskSection(info.RAM, info.Disk))
	b.WriteString("\n")

	b.WriteString(renderCPUSection(info.CPU))

	return b.String()
}

func renderCPUSection(cpu CPUInfo) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("● CPU Information"))
	b.WriteString("\n")

	if cpu.Model != "" {
		b.WriteString(fmt.Sprintf("  Model:  %s\n", cpu.Model))
	}
	if cpu.Count != "" {
		b.WriteString(fmt.Sprintf("  Count:  %s\n", cpu.Count))
	}
	b.WriteString(fmt.Sprintf("  Usage:  %s\n", cpu.Usage))

	return b.String()
}

func renderGPUSection(gpus []GPUInfo) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("● GPU Information"))
	b.WriteString("\n\n")

	for i := 0; i < len(gpus); i += 2 {
		var leftGPU, rightGPU string

		leftGPU = renderSingleGPU(gpus[i])

		if i+1 < len(gpus) {
			rightGPU = renderSingleGPU(gpus[i+1])
			row := lipgloss.JoinHorizontal(lipgloss.Top, leftGPU, "    ", rightGPU)
			b.WriteString(row)
		} else {
			b.WriteString(leftGPU)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func renderSingleGPU(gpu GPUInfo) string {
	var b strings.Builder

	vramTotalGB := float64(gpu.VRAMTotal) / 1024.0
	vramUsedGB := float64(gpu.VRAMUsed) / 1024.0
	vramPercent := 0.0
	if gpu.VRAMTotal > 0 {
		vramPercent = (float64(gpu.VRAMUsed) / float64(gpu.VRAMTotal)) * 100
	}

	gpuTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Render(fmt.Sprintf("GPU %s", gpu.Index))

	gpuName := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(gpu.Name)

	const barWidth = 50

	b.WriteString(fmt.Sprintf("  %s  %s\n", gpuTitle, gpuName))

	vramLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("VRAM")
	b.WriteString(fmt.Sprintf("  %s %.1f/%.1f GB (%.1f%%)\n", vramLabel, vramUsedGB, vramTotalGB, vramPercent))
	b.WriteString("  ")
	b.WriteString(renderProgressBar(vramPercent, barWidth, lipgloss.Color("39")))
	b.WriteString("\n")

	utilLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("Util")
	b.WriteString(fmt.Sprintf("  %s %d%%\n", utilLabel, gpu.Utilization))
	b.WriteString("  ")
	b.WriteString(renderProgressBar(float64(gpu.Utilization), barWidth, lipgloss.Color("208")))
	b.WriteString("\n")

	return b.String()
}

func renderRAMAndDiskSection(ram RAMInfo, disks []DiskInfo) string {
	ramSection := renderRAMSection(ram)
	diskSection := renderDiskSection(disks)

	return lipgloss.JoinHorizontal(lipgloss.Top, ramSection, "    ", diskSection) + "\n"
}

func renderRAMSection(ram RAMInfo) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("● RAM Information"))
	b.WriteString("\n")

	totalGB := float64(ram.Total) / 1024.0
	usedGB := float64(ram.Used) / 1024.0

	b.WriteString(fmt.Sprintf("  %.1f GB / %.1f GB (%.1f%%)\n", usedGB, totalGB, ram.UsagePercent))
	b.WriteString("  ")
	b.WriteString(renderProgressBar(ram.UsagePercent, 50, lipgloss.Color("10")))
	b.WriteString("\n")

	return b.String()
}

func renderDiskSection(disks []DiskInfo) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("● Disk Usage"))
	b.WriteString("\n")

	for _, disk := range disks {
		usageStr := strings.TrimSuffix(disk.UsagePercent, "%")
		usagePercent := 0.0
		if val, err := strconv.ParseFloat(usageStr, 64); err == nil {
			usagePercent = val
		}

		b.WriteString(fmt.Sprintf("  %s  %s  %s / %s (%s)\n",
			disk.Device, disk.MountPoint, disk.Used, disk.Size, disk.UsagePercent))

		b.WriteString("  ")

		var barColor lipgloss.Color
		if usagePercent >= 90 {
			barColor = lipgloss.Color("196") // Red
		} else if usagePercent >= 75 {
			barColor = lipgloss.Color("208") // Orange
		} else {
			barColor = lipgloss.Color("10") // Green
		}

		b.WriteString(renderProgressBar(usagePercent, 50, barColor))
		b.WriteString("\n\n")
	}

	return b.String()
}
