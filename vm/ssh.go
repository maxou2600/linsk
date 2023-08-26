package vm

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alessio/shellescape"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

func ParseSSHKeyScan(knownHosts []byte) (ssh.HostKeyCallback, error) {
	knownKeysMap := make(map[string][]byte)
	for _, line := range strings.Split(string(knownHosts), "\n") {
		if len(line) == 0 {
			continue
		}

		lineSplit := strings.Split(line, " ")
		if want, have := 3, len(lineSplit); want != have {
			return nil, fmt.Errorf("bad split ssh identity string length: want %v, have %v ('%v')", want, have, line)
		}

		b, err := base64.StdEncoding.DecodeString(lineSplit[2])
		if err != nil {
			return nil, errors.Wrap(err, "decode base64 public key")
		}

		knownKeysMap[lineSplit[1]] = b
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		knownKey, ok := knownKeysMap[key.Type()]
		if !ok {
			return fmt.Errorf("unknown key type '%v'", key.Type())
		}

		if !bytes.Equal(key.Marshal(), knownKey) {
			return fmt.Errorf("public key mismatch")
		}

		return nil
	}, nil
}

func (vi *Instance) scanSSHIdentity() ([]byte, error) {
	vi.resetSerialStdout()

	err := vi.writeSerial([]byte(`ssh-keyscan -H localhost; echo "SERIAL STATUS: $?"; rm /root/.ash_history` + "\n"))
	if err != nil {
		return nil, errors.Wrap(err, "write keyscan command to serial")
	}

	deadline := time.Now().Add(time.Second * 5)

	var ret bytes.Buffer

	for {
		select {
		case <-vi.ctx.Done():
			return nil, vi.ctx.Err()
		case <-time.After(time.Until(deadline)):
			return nil, fmt.Errorf("keyscan command timed out")
		case data := <-vi.serialStdoutCh:
			if len(data) == 0 {
				continue
			}

			prefix := []byte("SERIAL STATUS: ")
			if bytes.HasPrefix(data, prefix) {
				if len(data) == len(prefix) {
					return nil, fmt.Errorf("keyscan command status code did not show up")
				}

				if data[len(prefix)] != '0' {
					return nil, fmt.Errorf("non-zero keyscan command status code: '%v'", string(data[len(prefix)]))
				}

				return ret.Bytes(), nil
			} else if data[0] == '|' {
				ret.Write(data)
			}
		}
	}
}

func (vi *Instance) sshSetup() (ssh.Signer, error) {
	vi.resetSerialStdout()

	sshSigner, sshPublicKey, err := generateSSHKey()
	if err != nil {
		return nil, errors.Wrap(err, "generate ssh key")
	}

	cmd := `set -ex; do_setup () { sh -c "set -ex; ifconfig eth0 up; ifconfig lo up; udhcpc; mkdir -p ~/.ssh; echo ` + shellescape.Quote(string(sshPublicKey)) + ` > ~/.ssh/authorized_keys; rc-update add sshd; service sshd start"; echo "SERIAL STATUS: $?"; }; do_setup` + "\n"

	err = vi.writeSerial([]byte(cmd))
	if err != nil {
		return nil, errors.Wrap(err, "write ssh setup serial command")
	}

	deadline := time.Now().Add(time.Second * 5)

	stdOutErrBuf := bytes.NewBuffer(nil)

	for {
		select {
		case <-vi.ctx.Done():
			return nil, vi.ctx.Err()
		case <-time.After(time.Until(deadline)):
			return nil, fmt.Errorf("setup command timed out %v", getLogErrMsg(stdOutErrBuf.String()))
		case data := <-vi.serialStdoutCh:
			prefix := []byte("SERIAL STATUS: ")
			stdOutErrBuf.Write(data)
			if bytes.HasPrefix(data, prefix) {
				if len(data) == len(prefix) {
					return nil, fmt.Errorf("setup command status code did not show up")
				}

				if data[len(prefix)] != '0' {
					return nil, fmt.Errorf("non-zero setup command status code: '%v' %v", string(data[len(prefix)]), getLogErrMsg(stdOutErrBuf.String()))
				}

				return sshSigner, nil
			}
		}
	}
}

func generateSSHKey() (ssh.Signer, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate rsa private key")
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create signer from key")
	}

	return signer, ssh.MarshalAuthorizedKey(signer.PublicKey()), nil
}

func runSSHCmd(c *ssh.Client, cmd string) ([]byte, error) {
	sess, err := c.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "create new vm ssh session")
	}

	defer func() { _ = sess.Close() }()

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	sess.Stdout = stdout
	sess.Stderr = stderr

	err = sess.Run(cmd)
	if err != nil {
		return nil, wrapErrWithLog(err, "run cmd", stderr.String())
	}

	return stdout.Bytes(), nil
}
