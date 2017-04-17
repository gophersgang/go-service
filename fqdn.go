package service

import (
	"bytes"
	"os/exec"
)

func getFQDN() string {
	// #nosec
	out, err := exec.Command("/bin/hostname", "-f").Output()
	if err != nil {
		return "localhost"
	}
	return string(bytes.TrimSpace(out))
}
