## kellner - fast adhoc serving of packages

*kellner* scans a given directory for known packages and creates an index. It then 
acts as an httpd to be used by *opkg* or other package managers.

*kellner* is pretty fast in creating the index: It takes ~3.8s on a Core-I5 to scan 
4698 packages with a combined file size of 644Mbyte and providing the SHA1 and MD5
for each of the packages.

### Usage

	$> keller -root dir_full_of_packages/


    -bind=":8080": address to bind to
    -dump=false: just dump the package list and exit
    -md5=true: calculate md5 of scanned packages
    -root="": directory containing the packages
    -sha1=true: calculate sha1 of scanned packages
    -workers=4: number of workers


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