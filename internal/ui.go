package internal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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
	screen         Screen
	hosts          []SSHHost
	selectedHosts  []SSHHost
	currentHostIdx int
	list           list.Model
	spinner        spinner.Model
	clients        map[string]*SSHClient
	sysInfos       map[string]*SystemInfo
	lastUpdates    map[string]time.Time
	updateInterval time.Duration
	err            error
	width          int
	height         int
}

type TickMsg time.Time

type SystemInfoMsg struct {
	hostName string
	info     *SystemInfo
	err      error
}

type ConnectedMsg struct {
	hostName string
	client   *SSHClient
	err      error
}

type hostItem struct {
	host     SSHHost
	selected bool
}

func (h hostItem) FilterValue() string { return h.host.Name }
func (h hostItem) Title() string {
	prefix := "  "
	if h.selected {
		prefix = "✓ "
	}
	return prefix + h.host.Name
}
func (h hostItem) Description() string {
	if h.host.Hostname != "" {
		return fmt.Sprintf("  %s@%s:%s", h.host.User, censorHostname(h.host.Hostname), h.host.Port)
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

func InitialModel(hosts []SSHHost, updateInterval time.Duration) Model {
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem{host: h, selected: false}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select SSH Hosts to Monitor (Space to select, Enter to confirm)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		screen:         ScreenHostList,
		hosts:          hosts,
		list:           l,
		spinner:        s,
		clients:        make(map[string]*SSHClient),
		sysInfos:       make(map[string]*SystemInfo),
		lastUpdates:    make(map[string]time.Time),
		updateInterval: updateInterval,
	}
}

func (m *Model) updateListSelection() {
	items := m.list.Items()
	selectedMap := make(map[string]bool)
	for _, h := range m.selectedHosts {
		selectedMap[h.Name] = true
	}

	newItems := make([]list.Item, len(items))
	for i, item := range items {
		if hi, ok := item.(hostItem); ok {
			hi.selected = selectedMap[hi.host.Name]
			newItems[i] = hi
		}
	}
	m.list.SetItems(newItems)
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			for _, client := range m.clients {
				if client != nil {
					client.Close()
				}
			}
			return m, tea.Quit
		case " ":
			if m.screen == ScreenHostList {
				if item, ok := m.list.SelectedItem().(hostItem); ok {
					host := item.host
					found := false
					for i, h := range m.selectedHosts {
						if h.Name == host.Name {
							m.selectedHosts = append(m.selectedHosts[:i], m.selectedHosts[i+1:]...)
							found = true
							break
						}
					}
					if !found {
						m.selectedHosts = append(m.selectedHosts, host)
					}
					m.updateListSelection()
				}
			}
		case "enter":
			if m.screen == ScreenHostList {
				if len(m.selectedHosts) == 0 {
					if item, ok := m.list.SelectedItem().(hostItem); ok {
						m.selectedHosts = append(m.selectedHosts, item.host)
					}
				}
				if len(m.selectedHosts) > 0 {
					m.screen = ScreenConnecting
					return m, m.connectToHosts()
				}
			}
		case "n":
			if m.screen == ScreenDashboard && len(m.selectedHosts) > 1 {
				m.currentHostIdx = (m.currentHostIdx + 1) % len(m.selectedHosts)
				nextHost := m.selectedHosts[m.currentHostIdx]
				if m.clients[nextHost.Name] == nil {
					return m, m.connectToHost(nextHost)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-2)

	case ConnectedMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("failed to connect to %s: %w", msg.hostName, msg.err)
			m.screen = ScreenError
			return m, nil
		}
		m.clients[msg.hostName] = msg.client

		if m.screen == ScreenConnecting && len(m.clients) == 1 {
			m.screen = ScreenDashboard
			return m, tea.Batch(m.gatherAllSysInfo(), m.tick())
		}

		if m.screen == ScreenDashboard {
			return m, m.gatherSysInfoForHost(msg.hostName)
		}

	case SystemInfoMsg:
		if msg.err != nil {
			return m, nil
		}
		m.sysInfos[msg.hostName] = msg.info
		m.lastUpdates[msg.hostName] = time.Now()

	case TickMsg:
		// update every 10 seconds
		return m, tea.Batch(m.gatherAllSysInfo(), m.tick())
	}

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)

	if m.screen == ScreenHostList {
		var listCmd tea.Cmd
		m.list, listCmd = m.list.Update(msg)
		return m, tea.Batch(spinnerCmd, listCmd)
	}

	return m, spinnerCmd
}

func (m Model) View() string {
	switch m.screen {
	case ScreenHostList:
		listView := m.list.View()
		if len(m.selectedHosts) > 0 {
			selectedNames := make([]string, len(m.selectedHosts))
			for i, h := range m.selectedHosts {
				selectedNames[i] = h.Name
			}
			footer := fmt.Sprintf("\nSelected (%d): %s", len(m.selectedHosts), strings.Join(selectedNames, ", "))
			listView += lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(footer)
		}
		return listView

	case ScreenConnecting:
		return m.renderLoading("Connecting and gathering information...")

	case ScreenDashboard:
		if len(m.selectedHosts) > 0 && m.currentHostIdx < len(m.selectedHosts) {
			currentHost := m.selectedHosts[m.currentHostIdx]

			if m.clients[currentHost.Name] == nil || m.sysInfos[currentHost.Name] == nil {
				return m.renderLoading(fmt.Sprintf("Loading %s...", currentHost.Name))
			}

			sysInfo := m.sysInfos[currentHost.Name]
			lastUpdate := m.lastUpdates[currentHost.Name]

			hostIndicator := ""
			if len(m.selectedHosts) > 1 {
				hostIndicator = fmt.Sprintf(" [%d/%d]", m.currentHostIdx+1, len(m.selectedHosts))
			}
			return renderDashboard(currentHost.Name+hostIndicator, sysInfo, m.updateInterval, lastUpdate, m.width, m.height, len(m.selectedHosts) > 1)
		}
		return m.renderLoading("Initializing...")

	case ScreenError:
		return renderError(m.err)
	}

	return ""
}

func (m Model) connectToHosts() tea.Cmd {
	if len(m.selectedHosts) > 0 {
		return m.connectToHost(m.selectedHosts[0])
	}
	return nil
}

func (m Model) connectToHost(host SSHHost) tea.Cmd {
	return func() tea.Msg {
		client, err := NewSSHClient(host)
		return ConnectedMsg{hostName: host.Name, client: client, err: err}
	}
}

func (m Model) gatherAllSysInfo() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		h := host
		client := m.clients[h.Name]
		if client != nil {
			cmds = append(cmds, func() tea.Msg {
				info, err := GatherSystemInfo(client)
				return SystemInfoMsg{hostName: h.Name, info: info, err: err}
			})
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) gatherSysInfoForHost(hostName string) tea.Cmd {
	client := m.clients[hostName]
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		info, err := GatherSystemInfo(client)
		return SystemInfoMsg{hostName: hostName, info: info, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.updateInterval, func(t time.Time) tea.Msg {
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

func (m Model) renderLoading(message string) string {
	return boxStyle.Render(fmt.Sprintf("%s %s", m.spinner.View(), message))
}

func renderError(err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %v\n\nPress 'q' to quit", err))
}

func renderDashboard(hostName string, info *SystemInfo, updateInterval time.Duration, lastUpdate time.Time, width, height int, multiHost bool) string {
	var b strings.Builder

	title := fmt.Sprintf("  System Dashboard - %s  ", hostName)
	navHint := ""
	if multiHost {
		navHint = " | Press 'n' for next host"
	}
	subtitle := fmt.Sprintf("Last Updated: %s | Interval: %.0fs%s | Press 'q' to quit",
		lastUpdate.Format("15:04:05"), updateInterval.Seconds(), navHint)

	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(subtitle))
	b.WriteString("\n\n")

	b.WriteString(renderCPUSection(info.CPU))
	b.WriteString("\n")

	b.WriteString(renderRAMAndDiskSection(info.RAM, info.Disk))

	if len(info.GPUs) > 0 {
		b.WriteString("\n")
		b.WriteString(renderGPUSection(info.GPUs))
	}

	return b.String()
}

func renderCPUSection(cpu CPUInfo) string {
	var parts []string

	if cpu.Model != "" {
		parts = append(parts, cpu.Model)
	}
	if cpu.Count != "" {
		parts = append(parts, fmt.Sprintf("%s cores", cpu.Count))
	}
	parts = append(parts, fmt.Sprintf("Usage: %s", cpu.Usage))

	cpuInfo := strings.Join(parts, "  |  ")
	return headerStyle.Render("● CPU") + "  " + cpuInfo + "\n"
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

	return lipgloss.JoinHorizontal(lipgloss.Top, diskSection, "    ", ramSection) + "\n"
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
