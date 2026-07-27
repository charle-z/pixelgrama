package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

type fakeRuntimeOps struct {
	euids []int
	calls []string
}

func (f *fakeRuntimeOps) Geteuid() int {
	value := f.euids[0]
	f.euids = f.euids[1:]
	return value
}

func (f *fakeRuntimeOps) MkdirAll(path string, mode os.FileMode) error {
	f.calls = append(f.calls, fmt.Sprintf("mkdir:%s:%04o", path, mode))
	return nil
}

func (f *fakeRuntimeOps) Chmod(path string, mode os.FileMode) error {
	f.calls = append(f.calls, fmt.Sprintf("chmod:%s:%04o", path, mode))
	return nil
}

func (f *fakeRuntimeOps) Chown(path string, uid, gid int) error {
	f.calls = append(f.calls, fmt.Sprintf("chown:%s:%d:%d", path, uid, gid))
	return nil
}

func (f *fakeRuntimeOps) Setgroups(groups []int) error {
	f.calls = append(f.calls, fmt.Sprintf("setgroups:%v", groups))
	return nil
}

func (f *fakeRuntimeOps) Setgid(gid int) error {
	f.calls = append(f.calls, fmt.Sprintf("setgid:%d", gid))
	return nil
}

func (f *fakeRuntimeOps) Setuid(uid int) error {
	f.calls = append(f.calls, fmt.Sprintf("setuid:%d", uid))
	return nil
}

func TestPrepareRuntimeDropsRootBeforeDatabaseOpen(t *testing.T) {
	ops := &fakeRuntimeOps{euids: []int{0, 10001}}
	if err := prepareRuntime("/data/pixelgrama.db", ops); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"mkdir:/data:0750",
		"chmod:/data:0750",
		"chown:/data:10001:10001",
		"setgroups:[]",
		"setgid:10001",
		"setuid:10001",
	}
	if !reflect.DeepEqual(ops.calls, want) {
		t.Fatalf("calls = %#v, want %#v", ops.calls, want)
	}
}

func TestPrepareRuntimeKeepsConfiguredNonRootUser(t *testing.T) {
	ops := &fakeRuntimeOps{euids: []int{10001}}
	if err := prepareRuntime("/data/pixelgrama.db", ops); err != nil {
		t.Fatal(err)
	}
	want := []string{"mkdir:/data:0750"}
	if !reflect.DeepEqual(ops.calls, want) {
		t.Fatalf("calls = %#v, want %#v", ops.calls, want)
	}
}
