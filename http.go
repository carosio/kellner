// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"net/http"
	"os"
	"path/filepath"
)

func makeIndexHandler(root, cache string) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		var (
			path     = filepath.Join(root, r.URL.Path)
			baseName = filepath.Base(path)
		)

		switch baseName {
		case "Packages", "Packages.gz", "Packages.stamps":
			var cachedPath = filepath.Join(cache, r.URL.Path)
			http.ServeFile(w, r, cachedPath)
			return
		}

		var fi os.FileInfo
		var err error
		if fi, err = os.Stat(path); err != nil {
			http.NotFound(w, r)
			return
		}

		if fi.IsDir() {
			renderIndex(w, r, path, root, cache)
			return
		}

		http.ServeFile(w, r, path)
	})
}
