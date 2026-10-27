package tokens

import (
	"encoding/base64"

	sshagentcrypt "github.com/131/ssh-agent-crypt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/131/ssh-agent-totp/internal/i18n"
)

// AgentKey is a public key held by the SSH agent.
type AgentKey struct {
	Key         string `json:"key"`
	Comment     string `json:"comment"`
	Fingerprint string `json:"fingerprint"`
}

// AgentKeys lists the keys of the agent at address.
func AgentKeys(address string) ([]AgentKey, error) {
	if address == "" {
		return nil, i18n.Errorf("err.agent.none")
	}

	conn, err := sshagentcrypt.DialAgent(address)
	if err != nil {
		return nil, i18n.Errorf("err.agent.unreachable")
	}

	defer conn.Close()

	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return nil, i18n.Errorf("err.agent.list")
	}

	result := []AgentKey{}
	for _, key := range keys {
		line := key.Type() + " " + base64.StdEncoding.EncodeToString(key.Marshal())
		result = append(result, AgentKey{Key: line, Comment: key.Comment, Fingerprint: ssh.FingerprintSHA256(key)})
	}

	return result, nil
}

// AgentCrypt seals with ssh-agent-crypt through the agent at address.
func AgentCrypt(address func() string) Crypt {
	return Crypt{
		Encrypt: func(plain []byte, key string) (string, error) {
			return sshagentcrypt.Encrypt(plain, key, &sshagentcrypt.Options{AgentSocket: address()})
		},
		Decrypt: func(armored []byte) ([]byte, error) {
			return sshagentcrypt.Decrypt(armored, "", &sshagentcrypt.Options{AgentSocket: address()})
		},
	}
}
