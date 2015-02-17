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
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sync"
	"time"
)

func main() {

	var (
		nworkers          = flag.Int("workers", 4, "number of workers")
		bind              = flag.String("bind", ":8080", "address to bind to")
		root_name         = flag.String("root", "", "directory containing the ipks")
		dump_package_list = flag.Bool("dump", false, "just dump the package list and exit")
	)
	flag.Parse()

	if *bind == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -bind")
		os.Exit(1)
	}
	if *root_name == "" {
		fmt.Fprintf(os.Stderr, "usage error: missing / empty -root")
	}

	root, err := os.Open(*root_name)
	if err != nil {
		fmt.Printf("error: opening -root %q: %v\n", *root_name, err)
		os.Exit(1)
	}

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
			n := path.Join(*root_name, name)

			file, err := os.Open(n)
			if err != nil {
				log.Printf("openening %q: %v", n, err)
				return
			}
			defer file.Close()

			// TODO: make it optional
			md5er := md5.New()
			sha1er := sha1.New()
			tee := io.TeeReader(file, io.MultiWriter(md5er, sha1er))

			control, err := ExtractControlFromIpk(tee)
			if err != nil {
				log.Printf("error: extract pkg-info from %q: %v", n, err)
				return
			}

			ipkg := &Ipkg{Name: name, Control: control, Header: make(map[string]string)}

			if err := ipkg.ControlToHeader(control); err != nil {
				log.Printf("error: header parse error in %q: %v", n, err)
				return
			}

			// consume the rest of the file to calculate md5/sha1
			io.Copy(ioutil.Discard, tee)

			ipkg.FileInfo, _ = os.Lstat(n)
			ipkg.Md5 = hex.EncodeToString(md5er.Sum(nil))
			ipkg.Sha1 = hex.EncodeToString(sha1er.Sum(nil))
			ipkg.EnhanceHeader()

			collector <- ipkg
		}(entry)
	}
	jobs.Wait()
	close(collector)

	fmt.Fprintf(os.Stderr, "time to parse %d packages: %s\n", len(packages.Entries), time.Since(now))

	if *dump_package_list {
		os.Stdout.WriteString(packages.String())
		return
	}

	ServeHTTP(&packages, *root_name, *bind)
}
