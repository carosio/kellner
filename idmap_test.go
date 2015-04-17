// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"os"
	"testing"
	"time"
)

func TestFindMappingFile(t *testing.T) {

	id := "O=SolSys,OU=Earth,CN=snowflake"
	fs := &mockFS{
		fs: map[string]os.FileInfo{
			"ids/O=SolSys":                              mockFileDir(_MockDir),
			"ids/O=SolSys/level0":                       mockFileDir(_MockFile),
			"ids/O=SolSys/overlay":                      mockFileDir(_MockFile),
			"ids/O=SolSys,OU=Earth":                     mockFileDir(_MockDir),
			"ids/O=SolSys,OU=Earth/level1":              mockFileDir(_MockFile),
			"ids/O=SolSys,OU=Earth/overlay":             mockFileDir(_MockFile),
			"ids/O=SolSys,OU=Earth,CN=snowflake":        mockFileDir(_MockDir),
			"ids/O=SolSys,OU=Earth,CN=snowflake/level2": mockFileDir(_MockFile),
		},
		t: t,
	}

	samples := []struct{ in, mf, mp string }{
		// expect success
		{"/level2/file", "ids/O=SolSys,OU=Earth,CN=snowflake/level2", "/level2"},
		{"/level2/", "ids/O=SolSys,OU=Earth,CN=snowflake/level2", "/level2"},
		{"/level1/file", "ids/O=SolSys,OU=Earth/level1", "/level1"},
		{"/overlay/file", "ids/O=SolSys,OU=Earth/overlay", "/overlay"},
		{"/level0", "ids/O=SolSys/level0", "/level0"},

		// expect "failure" (indicated by "/" and the "toplevel" subject)
		{"/404", "ids/O=SolSys", "/"},
	}

	test := func(path, expected_mf, expected_path string) {
		mf, mp, fi, err := findMappingFile(path, "ids", id, ",", fs)
		t.Logf("findMappingFile(%q): %q %q %v %v", path, mf, mp, fi, err)
		if mf != expected_mf {
			t.Fatalf("findMappingFile(%q): expected mapFile to be %q, got %q", path, expected_mf, mf)
		}
		if mp != expected_path {
			t.Fatalf("findMappingFile(%q): expected mappedPath to be %q, got %q", path, expected_path, mp)
		}
	}

	for i := range samples {
		test(samples[i].in, samples[i].mf, samples[i].mp)
	}
}

// test the mocking .. test :)
func TestMockFS(t *testing.T) {

	fs := &mockFS{
		fs: map[string]os.FileInfo{
			"/":         mockFileDir(_MockDir),
			"/folder":   mockFileDir(_MockDir),
			"/folder/a": mockFileDir(_MockFile),
		},
		t: t,
	}

	fatal := func(name string, fi os.FileInfo, err error) {
		d := ""
		if fi != nil {
			d = " (no dir)"
		}
		t.Fatalf("expected to find %q, got %v%s", name, err, d)
	}

	if fi, err := fs.Stat("/"); err != nil || !fi.IsDir() {
		fatal("/", fi, err)
	}
	if fi, err := fs.Stat("/folder"); err != nil || !fi.IsDir() {
		fatal("/folder", fi, err)
	}
	if _, err := fs.Stat("/404"); err != os.ErrNotExist {
		t.Fatal("expected to not find %q, but found it?", "/bar")
	}

}

const (
	_MockFile = iota
	_MockDir
)

// mockFileDir is a os.FileInfo compatible interface which helps
// to test mockFS.
type mockFileDir int

func (m mockFileDir) Name() string       { return "" }
func (m mockFileDir) Size() int64        { return 0 }
func (m mockFileDir) Mode() os.FileMode  { return 0 }
func (m mockFileDir) ModTime() time.Time { return time.Now() }
func (m mockFileDir) IsDir() bool        { return (m == _MockDir) }
func (m mockFileDir) Sys() interface{}   { return m }

// mockFS implementes the fileProbe interface. it's only purpose
// is to test findMappingFile
type mockFS struct {
	fs map[string]os.FileInfo
	t  *testing.T
}

func (mock *mockFS) Stat(name string) (os.FileInfo, error) {
	var err error
	fi, exists := mock.fs[name]
	if !exists {
		err = os.ErrNotExist
	}
	mock.t.Logf("mockFS.Lstat(%q): %v %v", name, fi, err)
	return fi, err
}
