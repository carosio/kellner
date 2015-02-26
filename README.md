## kellner - fast adhoc serving of packages

*kellner* scans a given directory for software packages and creates an index.
It then acts as an adhoc httpd which serves the packages to *opkg* or other
package managers.

### Usage

    $> keller -root dir_full_of_packages/

    -bind=":8080":   address to bind to
    -dump=false:     just dump the package list and exit
    -md5=true:       calculate md5 of scanned packages
    -root="":        directory containing the packages
    -sha1=true:      calculate sha1 of scanned packages
    -version=false:  show version number
    -workers=4:      number of workers


### Building

Since *kellner* is written in go, you need a go compiler. Consult your OS how to
get one or go to http://golang.org/dl.

Once you have a working go compiler:

	$> cd kellner
	$> export GOPATH=`pwd`
	$> go get -v -d
	$> go build -v

You should now have the *kellner* binary in your working directory.

### Limitations

Right now *kellner*:

- supports only .ipk packages
- builds one index only (one would need multiple instances to serve architecture specific
  packages for example)


### Name

'Kellner' is the german term for 'waiter'. As such, a 'Kellner' serves /
delivers things listed on a menu. *kellner* delivers packages, based upon a
created index (the menu).

