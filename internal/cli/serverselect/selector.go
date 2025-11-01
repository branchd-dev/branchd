package serverselect

import (
	"fmt"

	"github.com/branchd-dev/branchd/internal/cli/config"
	"github.com/branchd-dev/branchd/internal/cli/userconfig"
	"github.com/manifoldco/promptui"
)

// ResolveServer determines which server to use based on the following priority:
// 1. If serverAlias flag is provided, use that server
// 2. If user has a selected server in their local config, use that
// 3. If only one server in project config, use that
// 4. Otherwise, prompt user to select a server interactively
func ResolveServer(projectConfig *config.Config, serverAlias string) (*config.Server, error) {
	// Priority 1: Use server alias if provided
	if serverAlias != "" {
		server, err := projectConfig.GetServerByAlias(serverAlias)
		if err != nil {
			return nil, err
		}
		return server, nil
	}

	// Priority 2: Use selected server from user config
	selectedIP, err := userconfig.GetSelectedServer()
	if err != nil {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}

	if selectedIP != "" {
		// Find server by IP in project config
		server, err := getServerByIP(projectConfig, selectedIP)
		if err != nil {
			// Selected server no longer exists in project config, clear it and continue
			_ = userconfig.SetSelectedServer("")
		} else {
			return server, nil
		}
	}

	// Priority 3: If only one server, use it automatically
	if len(projectConfig.Servers) == 1 {
		server := &projectConfig.Servers[0]
		// Save it as the selected server
		if err := userconfig.SetSelectedServer(server.IP); err != nil {
			// Don't fail if we can't save, just continue
			fmt.Printf("Warning: failed to save selected server: %v\n", err)
		}
		return server, nil
	}

	// Priority 4: Prompt user to select a server
	server, err := PromptServerSelection(projectConfig)
	if err != nil {
		return nil, err
	}

	// Save the selected server
	if err := userconfig.SetSelectedServer(server.IP); err != nil {
		// Don't fail if we can't save, just continue
		fmt.Printf("Warning: failed to save selected server: %v\n", err)
	}

	return server, nil
}

// PromptServerSelection shows an interactive prompt for the user to select a server
func PromptServerSelection(projectConfig *config.Config) (*config.Server, error) {
	if len(projectConfig.Servers) == 0 {
		return nil, fmt.Errorf("no servers configured in branchd.json")
	}

	// Create display labels for each server
	type serverOption struct {
		Label  string
		Server *config.Server
	}

	options := make([]serverOption, len(projectConfig.Servers))
	for i := range projectConfig.Servers {
		server := &projectConfig.Servers[i]
		label := fmt.Sprintf("%s (%s)", server.Alias, server.IP)
		options[i] = serverOption{
			Label:  label,
			Server: server,
		}
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "> {{ .Label | cyan }}",
		Inactive: "  {{ .Label }}",
		Selected: "{{ .Label | green }}",
	}

	prompt := promptui.Select{
		Label:     "Select a server",
		Items:     options,
		Templates: templates,
		Size:      10,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return nil, fmt.Errorf("server selection cancelled: %w", err)
	}

	return options[index].Server, nil
}

// getServerByIP finds a server in the config by its IP address
func getServerByIP(cfg *config.Config, ip string) (*config.Server, error) {
	for i := range cfg.Servers {
		if cfg.Servers[i].IP == ip {
			return &cfg.Servers[i], nil
		}
	}
	return nil, fmt.Errorf("server with IP '%s' not found in project config", ip)
}

// GetServerByIPOrAlias finds a server by IP address or alias
func GetServerByIPOrAlias(cfg *config.Config, ipOrAlias string) (*config.Server, error) {
	// First try by IP
	for i := range cfg.Servers {
		if cfg.Servers[i].IP == ipOrAlias {
			return &cfg.Servers[i], nil
		}
	}

	// Then try by alias
	for i := range cfg.Servers {
		if cfg.Servers[i].Alias == ipOrAlias {
			return &cfg.Servers[i], nil
		}
	}

	return nil, fmt.Errorf("server with IP or alias '%s' not found", ipOrAlias)
}
