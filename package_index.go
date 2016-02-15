// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"sync"
)

type packageIndex struct {
	sync.Mutex
	Entries map[string]*ipkArchive
}

func (pi *packageIndex) Len() int { return len(pi.Entries) }

// StringTo writes all control data and checksums
// to write 'w'. Essentially it creates a
// a 'Packages' file
func (pi *packageIndex) StringTo(w io.Writer) {
	for _, name := range pi.SortedNames() {
		entry := pi.Entries[name]
		entry.ControlAndChecksumTo(w)
		fmt.Fprintln(w)
	}
}

// StampsTo writes all timestamps to write 'w'
func (pi *packageIndex) StampsTo(w io.Writer) {
	for _, name := range pi.SortedNames() {
		entry := pi.Entries[name]
		fmt.Fprintf(w, "%d %s\n", entry.FileInfo.ModTime().Unix(), name)
	}
}

func (pi *packageIndex) String() string {
	buf := bytes.NewBuffer(nil)
	pi.StringTo(buf)
	return buf.String()
}

func (pi *packageIndex) SortedNames() []string {
	var names = make([]string, 0, len(pi.Entries))
	for _, pkg := range pi.Entries {
		names = append(names, pkg.Name)
	}
	sort.Strings(names)
	return names
}

func (pi *packageIndex) Add(name string, ipk *ipkArchive) {
	pi.Lock()
	pi.Entries[name] = ipk
	pi.Unlock()
}
