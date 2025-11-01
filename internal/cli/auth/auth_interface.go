package auth

// TokenStore defines the interface for token storage operations
// This allows us to mock the keyring in tests
type TokenStore interface {
	SaveToken(serverIP, token string) error
	LoadToken(serverIP string) (string, error)
	DeleteToken(serverIP string) error
}

// defaultTokenStore implements TokenStore using the OS keyring
type defaultTokenStore struct{}

var Default TokenStore = &defaultTokenStore{}

func (d *defaultTokenStore) SaveToken(serverIP, token string) error {
	return SaveToken(serverIP, token)
}

func (d *defaultTokenStore) LoadToken(serverIP string) (string, error) {
	return LoadToken(serverIP)
}

func (d *defaultTokenStore) DeleteToken(serverIP string) error {
	return DeleteToken(serverIP)
}
