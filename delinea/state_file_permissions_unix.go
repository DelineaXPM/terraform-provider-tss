//go:build !windows

package delinea

import "os"

func restrictStateFile(_ string, file *os.File) error {
	return file.Chmod(stateFileMode)
}

func syncStateDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
