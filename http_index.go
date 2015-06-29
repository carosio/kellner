// This file is part of *kellner*
//
// Copyright (C) 2015, Travelping GmbH <copyright@travelping.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
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
		<td class="col-link"><a href="{{.Href}}">{{.Name}}</a></td>
		<td class="col-modtime">{{.ModTime.Format "2006-01-02T15:04:05Z07:00" }}</td>
		<td class="col-size">{{.Size}}</td>
		<td class="col-descr"><a href="{{.Href}}.control" title="{{.RawDescr | html }}">{{.Descr}}</td>
	</tr>
{{end}}
	</tbody>
</table>

<footer>{{.Version}} - generated at {{.Date}}</footer>
`

var indexTemplate = template.Must(template.New("index").Parse(_Template))

type dirEntry struct {
	Name     string
	Href     string
	ModTime  time.Time
	Size     int64
	RawDescr string
	Descr    string
}

type dirEntryByName []dirEntry

func (a dirEntryByName) Len() int           { return len(a) }
func (a dirEntryByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a dirEntryByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

type renderCtx struct {
	Title       string
	Entries     []dirEntry
	SumFileSize int64
	Date        time.Time
	Version     string
}

func renderIndex(w http.ResponseWriter, r *http.Request, root, cache string) {

	var reqPath = filepath.Join(root, r.URL.Path)
	var ctx = renderCtx{
		Title:   r.URL.Path + " - kellner",
		Version: versionString,
		Date:    time.Now()}

	var dir, err = os.Open(reqPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer dir.Close()
	var entry os.FileInfo
	var entries []os.FileInfo

	entries, err = dir.Readdir(-1)

	if entry, err = os.Stat(filepath.Join(cache, r.URL.Path, "Packages")); err == nil {
		entries = append(entries, entry)
	}
	if entry, err = os.Stat(filepath.Join(cache, r.URL.Path, "Packages.gz")); err == nil {
		entries = append(entries, entry)
	}
	if entry, err = os.Stat(filepath.Join(cache, r.URL.Path, "Packages.stamps")); err == nil {
		entries = append(entries, entry)
	}

	ctx.Entries = make([]dirEntry, len(entries)+1)

	var i int
	for i, entry = range entries {
		ctx.Entries[i] = dirEntry{
			Name:    entry.Name(),
			Href:    path.Join(r.URL.Path, entry.Name()),
			ModTime: entry.ModTime(),
			Size:    int64(entry.Size()),
		}
		/* TODO: re-enable
		var raw, _ = ioutil.ReadFile(filepath.Join(cache, entry.Name()))
		ctx.Entries[i].RawDescr = string(raw)
		*/

		ctx.SumFileSize += entry.Size()
	}

	if reqPath == root {
		ctx.Entries = ctx.Entries[:len(ctx.Entries)-1]
	} else {
		ctx.Entries[len(ctx.Entries)-1] = dirEntry{
			Name:    "..",
			Href:    path.Join(r.URL.Path, ".."),
			ModTime: ctx.Date,
		}
	}
	sort.Sort(dirEntryByName(ctx.Entries))

	if err = indexTemplate.Execute(w, ctx); err != nil {
		log.Println("error: rendering %q: %v", r.URL.Path, err)
	}
}
