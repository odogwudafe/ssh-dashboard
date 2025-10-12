# SSH Dashboard

Monitor CPU, GPU, RAM, and disk usage on your remote servers with a live-updating terminal dashboard.

## Installation

### From Source

```bash
git clone https://github.com/AlpinDale/ssh-dashboard.git
cd ssh-dashboard
make install
```

This will install to `~/.local/bin`. Make sure this directory is in your PATH (it usually is):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Or use the install script which checks your PATH automatically:

```bash
./install.sh
```

### Prerequisites

- Go 1.21 or higher
- SSH access to remote hosts
- SSH keys loaded in your SSH agent

## Usage

Simply run:

```bash
ssh-dashboard
```

The tool will:
1. Scan your `~/.ssh/config` for available hosts
2. Present an interactive list to select from
3. Connect and display a live dashboard
4. Update stats every 10 seconds

Press `q` or `Ctrl+C` to quit.

## SSH Configuration

Make sure your `~/.ssh/config` is properly configured:

```
Host myserver
    HostName 192.168.1.100
    User username
    Port 22
    IdentityFile ~/.ssh/id_rsa

Host gpu-server
    HostName gpu.example.com
    User admin
    IdentityFile ~/.ssh/id_ed25519
```

### SSH Agent

The dashboard uses SSH agent for authentication. Make sure your keys are loaded:

```bash
ssh-add ~/.ssh/id_rsa
ssh-add ~/.ssh/id_ed25519

# verify
ssh-add -l
```

## Remote Requirements

The remote hosts should have these commands available:
- `lscpu` - CPU information
- `top` - CPU usage
- `free` - RAM information
- `df` - Disk usage
- `nvidia-smi` - GPU information (NVIDIA GPUs only)

Most Linux distributions include these by default.

## Development

### Build

```bash
make build
```

### Run

```bash
make run
```

### Build for Multiple Platforms

```bash
make build-all
```

This creates binaries for:
- Linux (amd64)
- macOS (amd64 and arm64)
- Windows (amd64) [not tested]

### Clean

```bash
make clean
```

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Troubleshooting

### Connection Issues
- Verify your SSH config is correct
- Test manual connection: `ssh hostname`
- Ensure SSH keys are loaded: `ssh-add -l`

### Missing GPU Information
- Verify NVIDIA drivers are installed: `ssh hostname nvidia-smi`
- The tool currently only supports NVIDIA GPUs

### Permission Denied
- Check SSH key permissions (should be 600)
- Verify the user has appropriate access rights

## Acknowledgments

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
