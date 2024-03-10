package feeding

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

var ErrNoQRCode = fmt.Errorf("No QR Code found")

// GetQRCode returns the data encoded in a QR code
func GetQRCode(in []byte) ([]byte, error) {
	tempFile, err := os.CreateTemp("", "QRCode.png")
	if err != nil {
		return nil, err
	}

	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	if _, err = tempFile.Write(in); err != nil {
		return nil, fmt.Errorf("Error writing to temp file: %w", err)
	}

	cmd := exec.Command("zbarimg", "--raw", "-q", tempFile.Name())
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Join(ErrNoQRCode, err)
	}

	return output, nil
}
