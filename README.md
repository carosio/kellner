## kellner - fast adhoc serving of packages

*kellner* scans a given directory for software packages and creates an index.
It then acts as an adhoc httpd which serves the packages to *opkg* or other
package managers.

### Usage

    $> keller -root dir_full_of_packages/

    -bind=":8080": address to bind to
    -dump=false: just dump the package list and exit
    -gzip=true: use 'gzip' to compress the package index. if false: use golang
    -idmap="": directory containing the client-mappings
    -log="": log to given filename
    -md5=true: calculate md5 of scanned packages
    -print-client-cert-id="": print client-id for given .cert and exit
    -require-client-cert=false: require a client-cert
    -root="": directory containing the packages
    -sha1=false: calculate sha1 of scanned packages
    -tls-cert="": PEM encoded ssl-cert
    -tls-client-ca-file="": file with PEM encoded list of ssl-certs containing the CAs
    -tls-key="": PEM encoded ssl-key
    -version=false: show version and exit
    -workers=4: number of workers


### Building

Since *kellner* is written in go, you need a go compiler. Consult your OS how to
get one or go to http://golang.org/dl.

Once you have a working go compiler:

	$> cd kellner
	$> export GOPATH=`pwd`:`pwd`/vendor
	$> go build -v

You should now have the *kellner* binary in your working directory.

### Feature: Identity mapping (serve content for specific clients)

If you need to provide different packages to different parties you might use
the 'identity mapping' feature of *kellner*. The mapping works by requiring
the clients to connect to *kellner* with a [client certificate][1]. The
certificate contains a "Subject":

    $> openssl x509 -noout -subject < client.crt
    subject= O=SolSys/OU=Earth/CN=sample

*kellner* uses the subject of the client certificate to lookup which packages
should be served to that specific client:

    $> kellner -idmap identities -root packages -require-client-cert \
        -tls-key s.key -tls-cert s.crt

Assume you have the following folders in your -root:

    $> ls -1 packages/
    all
    core2-64
    vmware
    secret

To map requests you need to create the `identities` directory. To get the
correct `client-id` from a given certificate, you could use mentioned openssl
command (and replace `/` with `,`) or you can use *kellner* directly:

    $> kellner -print-client-cert-id client.crt
    O=SolSys,OU=Earth,CN=sample

Next, create the mapping hierarchy:

    $> mkdir -p identities/O=SolSys,OU=Earth,CN=sample
    $> mkdir    identities/O=SolSys,OU=Earth
    $> mkdir    identities/O=SolSys

This is how to map requests:


Serve `packages/core2-64` as it is, for all certificates where the subject
starts with `O=SolSys,OU=Earth`:

    $> touch identities/O=SolSys,OU=Earth/core2-64


Serve `packages/secret` when requesting `/subset/Packages`:

    $> echo "secret" > identities/O=SolSys,OU=Earth,CN=sample/subset


Serve `packages/all` for all certificates where the subject starts with
`O=SolSys`:

    $> touch identities/O=SolSys/all


Disallow `O=SolSys,OU=Mars` from accessing `packages/all`:

    $> echo "deny" > identities/O=SolSys,OU=Mars/all


TL;DR:

    packages/all/*.ipk
    packages/core2-64/*.ipk
    packages/vmware/*.ipk
    packages/secret/*.ipk

    identities/O=SolSys,OU=Earth,CN=sample/subset  "secret" => packages/secret
    identities/O=SolSys,OU=Earth/core2-64          ""       => packages/core2-64
    identities/O=SolSys/all                        ""       => packages/all
    identities/O=SolSys,OU=Mars/all                "deny"   => 404



### Limitations

Right now *kellner*:

- supports only .ipk packages


### Name

'Kellner' is the german term for 'waiter'. As such, a 'Kellner' serves /
delivers things listed on a menu. *kellner* delivers packages, based upon a
created index (the menu).

