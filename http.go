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
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

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

var indexTemplate *template.Template

func init() {
	tmpl, err := template.New("index").Parse(_Template)
	if err != nil {
		panic(err)
	}
	indexTemplate = tmpl
}

func attachHTTPHandler(mux *http.ServeMux, packages *packageIndex, prefix, root string, gzipper gzWrite) {

	now := time.Now()

	packagesStamps := bytes.NewBuffer(nil)
	packagesContent := bytes.NewBuffer(nil)
	packagesContentGz := bytes.NewBuffer(nil)
	packages.StringTo(packagesContent)
	gzipper(packagesContentGz, bytes.NewReader(packagesContent.Bytes()))
	packages.StampsTo(packagesStamps)

	packagesHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			http.ServeContent(w, r, "Packages", now, bytes.NewReader(packagesContent.Bytes()))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		http.ServeContent(w, r, "Packages", now, bytes.NewReader(packagesContentGz.Bytes()))
	})

	packagesGzHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "Packages.gz", now, bytes.NewReader(packagesContentGz.Bytes()))
	})

	packagesStampsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "Packages.stamps", now, bytes.NewReader(packagesStamps.Bytes()))
	})

	indexHandler := func() http.Handler {

		names := packages.SortedNames()
		ctx := renderCtx{Title: prefix + " - kellner", Version: versionString, Date: time.Now()}

		const nMetaFiles = 3
		ctx.Entries = make([]dirEntry, len(names)+nMetaFiles)
		ctx.Entries[0] = dirEntry{Name: "Packages", ModTime: now, Size: int64(packagesContent.Len())}
		ctx.Entries[1] = dirEntry{Name: "Packages.gz", ModTime: now, Size: int64(packagesContentGz.Len())}
		ctx.Entries[2] = dirEntry{Name: "Packages.stamps", ModTime: now, Size: int64(packagesStamps.Len())}

		for i, name := range names {
			entry := packages.Entries[name]
			ctx.Entries[i+nMetaFiles] = entry.DirEntry()
			ctx.SumFileSize += entry.FileInfo.Size()
		}

		index, indexGz := ctx.render(indexTemplate)

		// the actual index handler
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, ".control") {
				entryName := r.URL.Path[:len(r.URL.Path)-8]
				entry, ok := packages.Entries[path.Base(entryName)]
				if !ok {
					http.NotFound(w, r)
					return
				}
				io.WriteString(w, entry.Control)
			} else if r.URL.Path == prefix || r.URL.Path == prefix+"/" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
					w.Write(index.Bytes())
					return
				}
				w.Header().Set("Content-Encoding", "gzip")
				w.Write(indexGz.Bytes())
			} else {
				http.ServeFile(w, r, path.Join(root, r.URL.Path))
			}
		})
	}()

	mux.Handle(prefix+"/", indexHandler)
	mux.Handle(prefix+"/Packages", packagesHandler)
	mux.Handle(prefix+"/Packages.gz", packagesGzHandler)
	mux.Handle(prefix+"/Packages.stamps", packagesStampsHandler)
}

func (ctx *renderCtx) render(tmpl *template.Template) (index, indexGz *bytes.Buffer) {

	index = bytes.NewBuffer(nil)
	if err := indexTemplate.Execute(index, ctx); err != nil {
		panic(err)
	}
	indexGz = bytes.NewBuffer(nil)
	gz := gzip.NewWriter(indexGz)
	gz.Write(index.Bytes())
	gz.Close()

	return index, indexGz
}

// based upon 'feeds' create a opkg-repository snippet:
//
//   src/gz name-ipks http://host:port/name
//   src/gz name2-ipks http://host:port/name2
//
// TODO: add that entry to the parent directory-handler "somehow"
func attachOpkgRepoSnippet(mux *http.ServeMux, mount string, feeds []string) {

	mux.Handle(mount, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		scheme := r.URL.Scheme
		if scheme == "" {
			scheme = "http://"
		}

		for _, muxPath := range feeds {
			repoName := strings.Replace(muxPath[1:], "/", "-", -1)
			fmt.Fprintf(w, "src/gz %s-ipks %s%s%s\n", repoName, scheme, r.Host, muxPath)
		}
	}))
}

const _ExtraLogKey = "kellner-log-data"

// wraps 'orig_handler' to log incoming http-request
func logRequests(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// NOTE: maybe a dopey idea: let the http-handlers attach logging
		// data to the request. pro: it hijacks a data structure meant to transport
		// external data
		//
		// only internal handlers are allowed to attach data to the
		// request to hand log-data over to this handler here. to make
		// sure external sources do not have control over our logs: delete
		// any existing data before starting the handler-chain.
		r.Header.Del(_ExtraLogKey)

		statusLog := logStatusCode{ResponseWriter: w}
		handler.ServeHTTP(&statusLog, r)
		if statusLog.Code == 0 {
			statusLog.Code = 200
		}

		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			log.Println(r.RemoteAddr, r.Method, statusLog.Code, r.Host, r.RequestURI, r.Header)
			return
		}

		// TODO: handle more than the first certificate
		clientID := clientIDByName(&r.TLS.PeerCertificates[0].Subject)
		log.Println(r.RemoteAddr, clientID, r.Method, statusLog.Code, r.Host, r.RequestURI, r.Header)
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
