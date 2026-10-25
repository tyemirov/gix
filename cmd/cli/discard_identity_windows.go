//go:build windows

package cli

import (
	"errors"
	"os"
	"syscall"
)

func discardFilesystemIdentity(path string, _ os.FileInfo) (discardFileIdentity, error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return discardFileIdentity{}, err
	}
	handle, err := syscall.CreateFile(pathPointer, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return discardFileIdentity{}, err
	}
	var information syscall.ByHandleFileInformation
	informationError := syscall.GetFileInformationByHandle(handle, &information)
	closeError := syscall.CloseHandle(handle)
	if err := errors.Join(informationError, closeError); err != nil {
		return discardFileIdentity{}, err
	}
	return discardFileIdentity{
		device: uint64(information.VolumeSerialNumber),
		inode:  uint64(information.FileIndexHigh)<<32 | uint64(information.FileIndexLow),
	}, nil
}
