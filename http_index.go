// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

const _Template = `<!doctype html>
<title>{{.Title}}</title>
<style type="text/css">
body { font-family: monospace }
td, th { padding: auto 2em }
.col-size { text-align: right }
.col-modtime { white-space: nowrap }
.col-descr { white-space: nowrap }
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

var indexTemplate = template.Must(template.New("index").Parse(_Template))

type dirEntry struct {
	Name     string
	ModTime  time.Time
	Size     int64
	RawDescr string
	Descr    string
}

type renderCtx struct {
	Title       string
	Entries     []dirEntry
	SumFileSize int64
	Date        time.Time
	Version     string
}

func renderIndex(w http.ResponseWriter, r *http.Request, path, root, cache string) {

	var ctx = renderCtx{
		Title:   path + " - kellner",
		Version: versionString,
		Date:    time.Now()}

	var rootDir, err = os.Open(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rootDir.Close()
	var entry os.FileInfo
	var entries []os.FileInfo

	entries, err = rootDir.Readdir(-1)

	if entry, err = os.Stat(filepath.Join(cache, "Packages")); err == nil {
		entries = append(entries, entry)
	}
	if entry, err = os.Stat(filepath.Join(cache, "Packages.gz")); err == nil {
		entries = append(entries, entry)
	}
	if entry, err = os.Stat(filepath.Join(cache, "Packages.stamps")); err == nil {
		entries = append(entries, entry)
	}

	ctx.Entries = make([]dirEntry, len(entries))

	var i int
	for i, entry = range entries {
		ctx.Entries[i] = dirEntry{
			Name:    entry.Name(),
			ModTime: ctx.Date,
			Size:    int64(entry.Size()),
		}
		var raw, _ = ioutil.ReadFile(filepath.Join(cache, entry.Name()))
		ctx.Entries[i].RawDescr = string(raw)

		ctx.SumFileSize += entry.Size()
	}

	if err = indexTemplate.Execute(w, ctx); err != nil {
		log.Println("error: rendering %q: %v", r.URL.Path, err)
	}
}
