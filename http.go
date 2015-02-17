package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os/exec"
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
td, th { padding: auto 2em }
.col-size { text-align: right }
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
		</tr>
	</thead>
	<tbody>
{{range .Entries}}
	<tr>
		<td class="col-link"><a href="{{.Name}}">{{.Name}}</a></td>
		<td class="col-modtime">{{.ModTime.Format "2006-02-01 15:04:05" }}</td>
		<td class="col-size">{{.Size}}</td>
		<td class="col-descr"><a href="{{.Name}}.control">{{.Descr}}</td>
	</tr>
{{end}}
	</tbody>
</table>`

func ServeHTTP(packages *PackageIndex, root, bind string) {

	now := time.Now()

	packages_content := bytes.NewBuffer(nil)
	packages_content_gz := bytes.NewBuffer(nil)
	packages.StringTo(packages_content)
	make_gz_gzip_pipe(packages_content_gz, packages_content.Bytes())

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
		}{Title: "opkg-list"}

		ctx.Entries = make([]DirEntry, len(names)+2)
		ctx.Entries[0] = DirEntry{Name: "Packages", ModTime: time.Now(), Size: int64(packages_content.Len())}
		ctx.Entries[1] = DirEntry{Name: "Packages.gz", ModTime: time.Now(), Size: int64(packages_content_gz.Len())}

		for i, name := range names {
			ipkg := packages.Entries[name]
			ctx.Entries[i+2] = ipkg.DirEntry()
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
	http.Handle("/", logger(index_handler))

	http.ListenAndServe(bind, nil)
}

func make_gz_golang_native(w io.Writer, data []byte) {
	gz, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	gz.Header.ModTime = time.Now()
	gz.Write(data)
	gz.Close()
}

// use a pipe to 'gzip' to create the gz in a way
// that opkg accepts the output. right now it's unclear
// why opkg explodes when it hits a golang-native-created
// gz file.
func make_gz_gzip_pipe(w io.Writer, data []byte) {
	cmd := exec.Command("gzip", "-9", "-c")
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = w
	cmd.Run()
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
