package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/branchd-dev/branchd/internal/cli/auth"
)

// Client represents an HTTP client for the Branchd API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new API client
func New(serverIP string) *Client {
	// Assume HTTPS by default (Caddy serves on 443)
	baseURL := fmt.Sprintf("https://%s", serverIP)

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// Skip TLS verification for self-signed certificates
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

// SetHTTPClient sets a custom HTTP client
func (c *Client) SetHTTPClient(httpClient *http.Client) {
	c.httpClient = httpClient
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token string `json:"token"`
	User  struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		IsAdmin bool   `json:"is_admin"`
	} `json:"user"`
}

// Login authenticates the user and returns a JWT token
func (c *Client) Login(email, password string) (*LoginResponse, error) {
	reqBody := LoginRequest{
		Email:    email,
		Password: password,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/auth/login", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(body))
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &loginResp, nil
}

// CreateBranchRequest represents the branch creation request
type CreateBranchRequest struct {
	Name string `json:"name"`
}

// CreateBranchResponse represents the branch creation response
type CreateBranchResponse struct {
	ID       string `json:"id"`
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
}

// CreateBranch creates a new database branch
func (c *Client) CreateBranch(serverIP, branchName string) (*CreateBranchResponse, error) {
	token, err := auth.LoadToken(serverIP)
	if err != nil {
		return nil, err
	}

	reqBody := CreateBranchRequest{
		Name: branchName,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/api/branches", c.baseURL),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create branch (status %d): %s", resp.StatusCode, string(body))
	}

	var branchResp CreateBranchResponse
	if err := json.NewDecoder(resp.Body).Decode(&branchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &branchResp, nil
}

// Branch represents a database branch
type Branch struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	CreatedBy     string `json:"created_by"`
	RestoreID     string `json:"restore_id"`
	RestoreName   string `json:"restore_name"`
	Port          int    `json:"port"`
	ConnectionURL string `json:"connection_url"`
}

// ListBranches returns all database branches
func (c *Client) ListBranches(serverIP string) ([]Branch, error) {
	token, err := auth.LoadToken(serverIP)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/api/branches", c.baseURL),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list branches (status %d): %s", resp.StatusCode, string(body))
	}

	var branches []Branch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return branches, nil
}

// DeleteBranch deletes a database branch by ID
func (c *Client) DeleteBranch(serverIP, branchID string) error {
	token, err := auth.LoadToken(serverIP)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		"DELETE",
		fmt.Sprintf("%s/api/branches/%s", c.baseURL, branchID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete branch (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateServer triggers a server update to the latest version
func (c *Client) UpdateServer(serverIP string) error {
	token, err := auth.LoadToken(serverIP)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/api/system/update", c.baseURL),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to trigger update (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// AnonRule represents an anonymization rule
type AnonRule struct {
	Table    string          `json:"table"`
	Column   string          `json:"column"`
	Template json.RawMessage `json:"template"`
	Type     string          `json:"type,omitempty"` // Optional: "text", "integer", "boolean", "null"
}

// UpdateAnonRulesRequest represents the bulk update request
type UpdateAnonRulesRequest struct {
	Rules []AnonRule `json:"rules"`
}

// UpdateAnonRules bulk replaces all anonymization rules
func (c *Client) UpdateAnonRules(serverIP string, rules []AnonRule) error {
	token, err := auth.LoadToken(serverIP)
	if err != nil {
		return err
	}

	reqBody := UpdateAnonRulesRequest{
		Rules: rules,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/api/anon-rules", c.baseURL),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update anon rules (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
