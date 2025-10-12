package internal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type SystemInfo struct {
	CPU  CPUInfo
	GPUs []GPUInfo
	RAM  RAMInfo
	Disk []DiskInfo
}

type CPUInfo struct {
	Model string
	Count string
	Usage string
}

type GPUInfo struct {
	Index       string
	Name        string
	VRAMTotal   int // in MB
	VRAMUsed    int // in MB
	Utilization int // percentage
	PowerDraw   int // in Watts
	PowerLimit  int // in Watts
	Temperature int // in Celsius
}

type RAMInfo struct {
	Total        int // in MB
	Used         int // in MB
	UsagePercent float64
}

type DiskInfo struct {
	Device       string
	Size         string
	Used         string
	Available    string
	UsagePercent string
	MountPoint   string
}

func GatherSystemInfo(client *SSHClient) (*SystemInfo, error) {
	info := &SystemInfo{}

	cpuInfo, err := getCPUInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}
	info.CPU = cpuInfo

	gpuInfo, _ := getGPUInfo(client)
	info.GPUs = gpuInfo

	ramInfo, err := getRAMInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get RAM info: %w", err)
	}
	info.RAM = ramInfo

	diskInfo, err := getDiskInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk info: %w", err)
	}
	info.Disk = diskInfo

	return info, nil
}

func getCPUInfo(client *SSHClient) (CPUInfo, error) {
	info := CPUInfo{}

	output, err := client.ExecuteCommand("lscpu | grep -E 'Model name|CPU\\(s\\):'")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Model name:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Model = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "CPU(s):") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Count = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	output, err = client.ExecuteCommand("top -bn1 | grep 'Cpu(s)' | sed 's/.*, *\\([0-9.]*\\)%* id.*/\\1/' | awk '{print 100 - $1}'")
	if err == nil {
		usage := strings.TrimSpace(output)
		if usage != "" {
			if val, err := strconv.ParseFloat(usage, 64); err == nil {
				info.Usage = fmt.Sprintf("%.1f%%", val)
			}
		}
	}

	if info.Usage == "" {
		info.Usage = "N/A"
	}

	return info, nil
}

func getGPUInfo(client *SSHClient) ([]GPUInfo, error) {
	// Try NVIDIA first
	_, err := client.ExecuteCommand("which nvidia-smi")
	if err == nil {
		return getNVIDIAGPUInfo(client)
	}

	// Try AMD
	_, err = client.ExecuteCommand("which amd-smi")
	if err == nil {
		return getAMDGPUInfoModern(client)
	}

	// Try older AMD tool
	_, err = client.ExecuteCommand("which rocm-smi")
	if err == nil {
		return getAMDGPUInfoLegacy(client)
	}

	return []GPUInfo{}, nil
}

func getNVIDIAGPUInfo(client *SSHClient) ([]GPUInfo, error) {
	output, err := client.ExecuteCommand("nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu,power.draw,power.limit,temperature.gpu --format=csv,noheader,nounits")
	if err != nil {
		return []GPUInfo{}, nil
	}

	var gpus []GPUInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 8 {
			gpu := GPUInfo{
				Index: strings.TrimSpace(parts[0]),
				Name:  strings.TrimSpace(parts[1]),
			}

			if val, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
				gpu.VRAMTotal = val
			}
			if val, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil {
				gpu.VRAMUsed = val
			}
			if val, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
				gpu.Utilization = val
			}
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64); err == nil {
				gpu.PowerDraw = int(val)
			}
			if val, err := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64); err == nil {
				gpu.PowerLimit = int(val)
			}
			if val, err := strconv.Atoi(strings.TrimSpace(parts[7])); err == nil {
				gpu.Temperature = val
			}

			gpus = append(gpus, gpu)
		}
	}

	return gpus, nil
}

func getAMDGPUInfoModern(client *SSHClient) ([]GPUInfo, error) {
	staticOutput, err := client.ExecuteCommand("amd-smi static --json 2>/dev/null")
	if err != nil {
		return []GPUInfo{}, nil
	}

	metricsOutput, err := client.ExecuteCommand("amd-smi metric --usage --power --temperature --mem-usage --json 2>/dev/null")
	if err != nil {
		return []GPUInfo{}, nil
	}

	var staticData struct {
		GPUData []struct {
			GPU  int `json:"gpu"`
			ASIC struct {
				MarketName string `json:"market_name"`
			} `json:"asic"`
			VRAM struct {
				Size struct {
					Value int    `json:"value"`
					Unit  string `json:"unit"`
				} `json:"size"`
			} `json:"vram"`
		} `json:"gpu_data"`
	}

	var metricsData struct {
		GPUData []struct {
			GPU   int `json:"gpu"`
			Usage struct {
				GFXActivity struct {
					Value int `json:"value"`
				} `json:"gfx_activity"`
			} `json:"usage"`
			Power struct {
				SocketPower struct {
					Value int `json:"value"`
				} `json:"socket_power"`
			} `json:"power"`
			Temperature struct {
				Hotspot struct {
					Value int `json:"value"`
				} `json:"hotspot"`
			} `json:"temperature"`
			MemUsage struct {
				TotalVRAM struct {
					Value int `json:"value"`
				} `json:"total_vram"`
				UsedVRAM struct {
					Value int `json:"value"`
				} `json:"used_vram"`
			} `json:"mem_usage"`
		} `json:"gpu_data"`
	}

	if err := json.Unmarshal([]byte(staticOutput), &staticData); err != nil {
		return []GPUInfo{}, nil
	}

	if err := json.Unmarshal([]byte(metricsOutput), &metricsData); err != nil {
		return []GPUInfo{}, nil
	}

	var gpus []GPUInfo
	for i, static := range staticData.GPUData {
		if i >= len(metricsData.GPUData) {
			break
		}
		metrics := metricsData.GPUData[i]

		gpu := GPUInfo{
			Index:       fmt.Sprintf("%d", static.GPU),
			Name:        static.ASIC.MarketName,
			VRAMTotal:   metrics.MemUsage.TotalVRAM.Value,
			VRAMUsed:    metrics.MemUsage.UsedVRAM.Value,
			Utilization: metrics.Usage.GFXActivity.Value,
			PowerDraw:   metrics.Power.SocketPower.Value,
			PowerLimit:  500, // AMD doesn't always report this, so we use a conservative estimate
			Temperature: metrics.Temperature.Hotspot.Value,
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

func getAMDGPUInfoLegacy(client *SSHClient) ([]GPUInfo, error) {
	output, err := client.ExecuteCommand("rocm-smi --showproductname --showmeminfo vram --showuse -t -P --csv 2>/dev/null")
	if err != nil {
		return []GPUInfo{}, nil
	}

	var gpus []GPUInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 2 {
		return []GPUInfo{}, nil
	}

	for i, line := range lines[1:] {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}

		gpu := GPUInfo{
			Index: fmt.Sprintf("%d", i),
		}

		if val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
			gpu.Temperature = int(val)
		}

		if val, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil {
			gpu.PowerDraw = int(val)
		}

		if val, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
			gpu.Utilization = val
		}

		if val, err := strconv.Atoi(strings.TrimSpace(parts[6])); err == nil {
			memOutput, err := client.ExecuteCommand(fmt.Sprintf("rocm-smi -d %d --showmeminfo vram --csv 2>/dev/null | grep -i 'Total VRAM'", i))
			if err == nil {
				memParts := strings.Split(memOutput, ",")
				if len(memParts) >= 2 {
					vramStr := strings.TrimSpace(memParts[1])
					vramStr = strings.TrimSuffix(vramStr, " MB")
					if totalVRAM, err := strconv.Atoi(strings.TrimSpace(vramStr)); err == nil {
						gpu.VRAMTotal = totalVRAM
						gpu.VRAMUsed = (totalVRAM * val) / 100
					}
				}
			}
		}

		if len(parts) >= 12 {
			series := strings.TrimSpace(parts[10])
			model := strings.TrimSpace(parts[11])
			if series != "" {
				gpu.Name = series
				if model != "" && model != series {
					gpu.Name = fmt.Sprintf("%s (%s)", series, model)
				}
			}
		}

		gpu.PowerLimit = 500 // conservative estimate

		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

func getRAMInfo(client *SSHClient) (RAMInfo, error) {
	info := RAMInfo{}

	output, err := client.ExecuteCommand("free -m | grep Mem:")
	if err != nil {
		return info, err
	}

	parts := strings.Fields(output)
	if len(parts) >= 3 {
		if val, err := strconv.Atoi(parts[1]); err == nil {
			info.Total = val
		}
		if val, err := strconv.Atoi(parts[2]); err == nil {
			info.Used = val
		}

		if info.Total > 0 {
			info.UsagePercent = (float64(info.Used) / float64(info.Total)) * 100
		}
	}

	return info, nil
}

func getDiskInfo(client *SSHClient) ([]DiskInfo, error) {
	output, err := client.ExecuteCommand("df -h | grep -E '^/dev/'")
	if err != nil {
		return nil, err
	}

	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 6 {
			disk := DiskInfo{
				Device:       parts[0],
				Size:         parts[1],
				Used:         parts[2],
				Available:    parts[3],
				UsagePercent: parts[4],
				MountPoint:   parts[5],
			}
			disks = append(disks, disk)
		}
	}

	return disks, nil
}
