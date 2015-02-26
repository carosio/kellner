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
	"compress/gzip"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"strings"
	"text/template"
	"time"
)

type DirEntry struct {
	Name     string
	ModTime  time.Time
	Size     int64
	RawDescr string
	Descr    string
}

const TEMPLATE = `<!doctype html>
<title>{{.Title}}</title>
<style type="text/css">
body { font-family: monospace }
td, th { padding: auto 2em }
.col-size { text-align: right }
.col-modtime { white-space: nowrap }
footer { margin-top: 1em; padding-top: 1em; border-top: 1px dotted silver }
</style>

<p>
This repository contains {{.Entries|len}} packages with an accumulated size of {{.SumFileSize}} bytes.
</p>
<table>
	<thead>
		<tr>
			<th>Name</th>
			<th>Last Modified</th>
			<th>Size</th>
			<th>Description</th>
		</tr>
	</thead>
	<tbody>
{{range .Entries}}
	<tr>
		<td class="col-link"><a href="{{.Name}}">{{.Name}}</a></td>
		<td class="col-modtime">{{.ModTime.Format "2006-01-02T15:04:05Z07:00" }}</td>
		<td class="col-size">{{.Size}}</td>
		<td class="col-descr"><a href="{{.Name}}.control" title="{{.RawDescr | html }}">{{.Descr}}</td>
	</tr>
{{end}}
	</tbody>
</table>

<footer>{{.Version}} - generated at {{.Date}}</footer>
`

func ServeHTTP(packages *PackageIndex, root string, gzipper Gzipper, listen net.Listener) {

	now := time.Now()

	packages_stamps := bytes.NewBuffer(nil)
	packages_content := bytes.NewBuffer(nil)
	packages_content_gz := bytes.NewBuffer(nil)
	packages.StringTo(packages_content)
	gzipper(packages_content_gz, bytes.NewReader(packages_content.Bytes()))
	packages.StampsTo(packages_stamps)

	packages_handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			http.ServeContent(w, r, "Packages", now, bytes.NewReader(packages_content.Bytes()))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		http.ServeContent(w, r, "Packages", now, bytes.NewReader(packages_content_gz.Bytes()))
	})

	packages_gz_handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "Packages.gz", now, bytes.NewReader(packages_content_gz.Bytes()))
	})

	packages_stamps_handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "Packages.stamps", now, bytes.NewReader(packages_stamps.Bytes()))
	})

	index_handler := func() http.Handler {

		tmpl, err := template.New("index").Parse(TEMPLATE)
		if err != nil {
			panic(err)
		}

		names := packages.SortedNames()

		ctx := struct {
			Title       string
			Entries     []DirEntry
			SumFileSize int64
			Date        time.Time
			Version     string
		}{Title: "opkg-list", Version: VERSION, Date: now}

		const n_meta_files = 3
		ctx.Entries = make([]DirEntry, len(names)+n_meta_files)
		ctx.Entries[0] = DirEntry{Name: "Packages", ModTime: now, Size: int64(packages_content.Len())}
		ctx.Entries[1] = DirEntry{Name: "Packages.gz", ModTime: now, Size: int64(packages_content_gz.Len())}
		ctx.Entries[2] = DirEntry{Name: "Packages.stamps", ModTime: now, Size: int64(packages_stamps.Len())}

		for i, name := range names {
			ipkg := packages.Entries[name]
			ctx.Entries[i+n_meta_files] = ipkg.DirEntry()
			ctx.SumFileSize += ipkg.FileInfo.Size()
		}

		index := bytes.NewBuffer(nil)
		if err := tmpl.Execute(index, &ctx); err != nil {
			panic(err)
		}
		index_gz := bytes.NewBuffer(nil)
		gz := gzip.NewWriter(index_gz)
		gz.Write(index.Bytes())
		gz.Close()

		// the actual index handler
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, ".control") {
				ipkg_name := r.URL.Path[:len(r.URL.Path)-8]
				ipkg, ok := packages.Entries[path.Base(ipkg_name)]
				if !ok {
					http.NotFound(w, r)
					return
				}
				io.WriteString(w, ipkg.Control)
			} else if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
					w.Write(index.Bytes())
					return
				}
				w.Header().Set("Content-Encoding", "gzip")
				w.Write(index_gz.Bytes())
			} else {
				http.ServeFile(w, r, path.Join(root, r.URL.Path))
			}
		})
	}()

	http.Handle("/Packages", logger(packages_handler))
	http.Handle("/Packages.gz", logger(packages_gz_handler))
	http.Handle("/Packages.stamps", logger(packages_stamps_handler))
	http.Handle("/", logger(index_handler))

	http.Serve(listen, nil)
}

// wraps 'orig_handler' to log incoming http-request
func logger(orig_handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status_log := logStatusCode{ResponseWriter: w}
		orig_handler.ServeHTTP(&status_log, r)
		if status_log.Code == 0 {
			status_log.Code = 200
		}
		log.Println(r.RemoteAddr, r.Method, status_log.Code, r.Host, r.RequestURI, r.Header)
	})
}

//
// small helper to intercept the http-statuscode written
// to the original http.ResponseWriter
type logStatusCode struct {
	http.ResponseWriter
	Code int
}

func (w *logStatusCode) WriteHeader(code int) {
	w.Code = code
	w.ResponseWriter.WriteHeader(code)
}
