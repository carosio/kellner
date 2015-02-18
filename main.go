package main

// *kellner* scans package files in a given directory
// and builds a Packages.gz file on the fly. it then serves the
// Packages.gz and the .ipk files by the built-in httpd
// and is ready to be used from opkg
//
// related tools:
// * https://github.com/17twenty/opkg-scanpackages
// * opkg-make-index from the opkg-utils collection

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"sync"
	"time"
)

func main() {

	var (
		nworkers          = flag.Int("workers", 4, "number of workers")
		bind              = flag.String("bind", ":8080", "address to bind to")
		root_name         = flag.String("root", "", "directory containing the packages")
		dump_package_list = flag.Bool("dump", false, "just dump the package list and exit")
		add_md5           = flag.Bool("md5", true, "calculate md5 of scanned packages")
		add_sha1          = flag.Bool("sha1", true, "calculate sha1 of scanned packages")
		use_gzip          = flag.Bool("gzip", true, "use 'gzip' to compress the package index. if false: use golang")
		listen            net.Listener
	)

	flag.Parse()

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind\n")
		os.Exit(1)
	}
	if !*dump_package_list {
		l, err := net.Listen("tcp", *bind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: binding to %q failed: %v\n", *bind, err)
			os.Exit(1)
		}
		listen = l
		log.Println("listen to", l.Addr())
	}

	if *root_name == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root")
	}

	root, err := os.Open(*root_name)
	if err != nil {
		fmt.Printf("error: opening -root %q: %v\n", *root_name, err)
		os.Exit(1)
	}

	log.Println("start building index from", *root_name)
	now := time.Now()
	entries, err := root.Readdirnames(-1)
	if err != nil {
		fmt.Printf("error: reading dir entries from -root %q: %v\n", *root_name, err)
		os.Exit(1)
	}

	//
	// create package list
	//
	packages := PackageIndex{Entries: make(map[string]*Ipkg)}
	collector := packages.CollectIpkgs(*nworkers)
	jobs := sync.WaitGroup{}

	for _, entry := range entries {
		if path.Ext(entry) != ".ipk" {
			continue
		}
		jobs.Add(1)
		go func(name string) {
			defer jobs.Done()

			var (
				full_name = path.Join(*root_name, name)
				file      *os.File
				writer    []io.Writer = make([]io.Writer, 0, 3)
				err       error
				md5er     hash.Hash
				sha1er    hash.Hash
			)

			file, err = os.Open(full_name)
			if err != nil {
				log.Printf("openening %q: %v", full_name, err)
				return
			}
			defer file.Close()

			writer = append(writer, ioutil.Discard)
			if *add_md5 {
				md5er = md5.New()
				writer = append(writer, md5er)
			}
			if *add_sha1 {
				sha1er = sha1.New()
				writer = append(writer, sha1er)
			}

			tee := io.TeeReader(file, io.MultiWriter(writer...))

			control, err := ExtractControlFromIpk(tee)
			if err != nil {
				log.Printf("error: extract pkg-info from %q: %v", full_name, err)
				return
			}

			ipkg := &Ipkg{Name: name, Control: control, Header: make(map[string]string)}

			if err := ipkg.ControlToHeader(control); err != nil {
				log.Printf("error: header parse error in %q: %v", full_name, err)
				return
			}

			// consume the rest of the file to calculate md5/sha1
			io.Copy(ioutil.Discard, tee)

			ipkg.FileInfo, _ = os.Lstat(full_name)
			if md5er != nil {
				ipkg.Md5 = hex.EncodeToString(md5er.Sum(nil))
			}
			if sha1er != nil {
				ipkg.Sha1 = hex.EncodeToString(sha1er.Sum(nil))
			}
			ipkg.EnhanceHeader()

			collector <- ipkg
		}(entry)
	}
	jobs.Wait()
	close(collector)

	log.Println("done building index")
	log.Printf("time to parse %d packages: %s\n", len(packages.Entries), time.Since(now))

	if *dump_package_list {
		os.Stdout.WriteString(packages.String())
		return
	}

	gzipper := Gzipper(GzGzipPipe)
	if !*use_gzip {
		gzipper = GzGolang
	}

	ServeHTTP(&packages, *root_name, gzipper, listen)
}
