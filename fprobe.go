// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import "os"

// fileProbe is a tiny helper to mock os.Stat() in tests, use
// fsFileProbe for real code
type fileProbe interface {
	Stat(name string) (os.FileInfo, error)
}

// fsFileProbe implements fileProbe
type fsFileProbe struct{}

func (_ *fsFileProbe) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
