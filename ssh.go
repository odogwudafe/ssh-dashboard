package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type SSHHost struct {
	Name         string
	Hostname     string
	User         string
	Port         string
	IdentityFile string
}

type SSHClient struct {
	client *ssh.Client
	config *SSHHost
}

func ParseSSHConfig(configPath string) ([]SSHHost, error) {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		configPath = filepath.Join(home, ".ssh", "config")
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var hosts []SSHHost
	var currentHost *SSHHost

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		if key == "host" {
			if currentHost != nil && !strings.Contains(currentHost.Name, "*") && !strings.Contains(currentHost.Name, "?") {
				hosts = append(hosts, *currentHost)
			}

			currentHost = &SSHHost{
				Name: value,
				Port: "22",
			}
		} else if currentHost != nil {
			switch key {
			case "hostname":
				currentHost.Hostname = value
			case "user":
				currentHost.User = value
			case "port":
				currentHost.Port = value
			case "identityfile":
				currentHost.IdentityFile = expandPath(value)
			}
		}
	}

	if currentHost != nil && !strings.Contains(currentHost.Name, "*") && !strings.Contains(currentHost.Name, "?") {
		hosts = append(hosts, *currentHost)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return hosts, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func NewSSHClient(host SSHHost) (*SSHClient, error) {
	// Set defaults
	if host.Hostname == "" {
		host.Hostname = host.Name
	}
	if host.User == "" {
		host.User = os.Getenv("USER")
	}
	if host.Port == "" {
		host.Port = "22"
	}

	var authMethods []ssh.AuthMethod
	var debugInfo []string

	fmt.Fprintf(os.Stderr, "[DEBUG] Trying SSH agent...\n")
	agentAuth, agentErr := sshAgentAuth()
	if agentErr == nil {
		authMethods = append(authMethods, agentAuth)
		debugInfo = append(debugInfo, "SSH agent")
		fmt.Fprintf(os.Stderr, "[DEBUG] SSH agent available, using agent only\n")
	} else {
		fmt.Fprintf(os.Stderr, "[DEBUG] SSH agent error: %v\n", agentErr)

		fmt.Fprintf(os.Stderr, "[DEBUG] Falling back to file-based authentication\n")

		if host.IdentityFile != "" {
			fmt.Fprintf(os.Stderr, "[DEBUG] Trying identity file: %s\n", host.IdentityFile)
			if keyAuth, err := publicKeyAuth(host.IdentityFile); err == nil {
				authMethods = append(authMethods, keyAuth)
				debugInfo = append(debugInfo, fmt.Sprintf("key: %s", host.IdentityFile))
				fmt.Fprintf(os.Stderr, "[DEBUG] Identity file loaded successfully\n")
			} else {
				fmt.Fprintf(os.Stderr, "[DEBUG] Identity file error: %v\n", err)
			}
		}

		home, err := os.UserHomeDir()
		if err == nil {
			defaultKeys := []string{
				filepath.Join(home, ".ssh", "id_rsa"),
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
			}
			for _, keyPath := range defaultKeys {
				fmt.Fprintf(os.Stderr, "[DEBUG] Trying default key: %s\n", keyPath)
				if keyAuth, err := publicKeyAuth(keyPath); err == nil {
					authMethods = append(authMethods, keyAuth)
					debugInfo = append(debugInfo, fmt.Sprintf("key: %s", keyPath))
					fmt.Fprintf(os.Stderr, "[DEBUG] Default key loaded successfully\n")
				} else {
					fmt.Fprintf(os.Stderr, "[DEBUG] Default key error: %v\n", err)
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Total auth methods: %d (%v)\n", len(authMethods), debugInfo)

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods available")
	}

	config := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%s", host.Hostname, host.Port)
	fmt.Fprintf(os.Stderr, "[DEBUG] Connecting to %s as user %s\n", addr, host.User)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Connection failed: %v\n", err)
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Connection successful!\n")

	return &SSHClient{
		client: client,
		config: &host,
	}, nil
}

func publicKeyAuth(keyPath string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// if it's an encrypted key error, we can't handle it without a passphrase
		// just return the error - SSH agent should handle these keys
		return nil, err
	}

	pubKey := signer.PublicKey()
	fingerprint := ssh.FingerprintSHA256(pubKey)
	fmt.Fprintf(os.Stderr, "[DEBUG]   Key type: %s, Fingerprint: %s\n", pubKey.Type(), fingerprint)

	return ssh.PublicKeys(signer), nil
}

func sshAgentAuth() (ssh.AuthMethod, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)

	keys, err := agentClient.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list agent keys: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[DEBUG]   Agent has %d key(s):\n", len(keys))
	for i, key := range keys {
		fmt.Fprintf(os.Stderr, "[DEBUG]     %d. %s %s\n", i+1, key.Type(), key.Comment)
	}

	return ssh.PublicKeysCallback(agentClient.Signers), nil
}

func (c *SSHClient) ExecuteCommand(cmd string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), err
	}

	return string(output), nil
}

func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
