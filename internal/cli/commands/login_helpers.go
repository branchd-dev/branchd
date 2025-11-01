package commands

import "github.com/branchd-dev/branchd/internal/cli/auth"

// defaultTokenStore wraps the auth package for production use
type defaultTokenStore struct{}

func (d *defaultTokenStore) SaveToken(serverIP, token string) error {
	return auth.SaveToken(serverIP, token)
}
