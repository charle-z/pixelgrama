package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const runtimeID = 10001

type runtimeOps interface {
	Geteuid() int
	MkdirAll(string, os.FileMode) error
	Chmod(string, os.FileMode) error
	Chown(string, int, int) error
	Setgroups([]int) error
	Setgid(int) error
	Setuid(int) error
}

type systemRuntimeOps struct{}

func (systemRuntimeOps) Geteuid() int { return os.Geteuid() }

func (systemRuntimeOps) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (systemRuntimeOps) Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func (systemRuntimeOps) Chown(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

func (systemRuntimeOps) Setgroups(groups []int) error {
	return syscall.Setgroups(groups)
}

func (systemRuntimeOps) Setgid(gid int) error {
	return syscall.Setgid(gid)
}

func (systemRuntimeOps) Setuid(uid int) error {
	return syscall.Setuid(uid)
}

func prepareRuntime(databasePath string, ops runtimeOps) error {
	directory := filepath.Dir(databasePath)
	if err := ops.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if ops.Geteuid() != 0 {
		return nil
	}
	if err := ops.Chmod(directory, 0o750); err != nil {
		return fmt.Errorf("set data directory mode: %w", err)
	}
	if err := ops.Chown(directory, runtimeID, runtimeID); err != nil {
		return fmt.Errorf("set data directory owner: %w", err)
	}
	if err := ops.Setgroups([]int{}); err != nil {
		return fmt.Errorf("clear supplementary groups: %w", err)
	}
	if err := ops.Setgid(runtimeID); err != nil {
		return fmt.Errorf("drop group privileges: %w", err)
	}
	if err := ops.Setuid(runtimeID); err != nil {
		return fmt.Errorf("drop user privileges: %w", err)
	}
	if uid := ops.Geteuid(); uid != runtimeID {
		return fmt.Errorf("effective uid is %d after privilege drop", uid)
	}
	return nil
}
