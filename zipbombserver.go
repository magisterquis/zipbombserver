// Zipbombserver erves up never-ending compressed zeros via http
package main

/*
 * decompressionbombserver.go
 * Serves up decompression bombs
 * By J. Stuart McMurray
 * Created 20170517
 * Last Modified 20170524
 */

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"os/signal"
	"runtime"
	"strconv"

	humanize "github.com/dustin/go-humanize"
)

// BOMBLET holds a compressed chunk of 0's
var BOMBLET []byte

func main() {
	var (
		doTLS = flag.Bool(
			"https",
			false,
			"Serve HTTPS instead of plaintext HTTP",
		)
		laddr = flag.String(
			"l",
			"0.0.0.0:80",
			"Listen address or socket path",
		)
		doFCGI = flag.Bool(
			"fcgi",
			false,
			"Serve via FCGI",
		)
		bombletSize = flag.String(
			"size",
			"10MB",
			"Bomblet size, `pre-compressed`",
		)
		sockPerm = flag.String(
			"perm",
			"660",
			"Octal `permissions` for FCGI socket",
		)
		certf = flag.String(
			"cert",
			"cert.pem",
			"TLS certificate `file`",
		)
		keyf = flag.String(
			"key",
			"key.pem",
			"TLS key `file`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %v [options]

Listens for HTTP or FastCGI requests, and serves up a never-ending stream of
compressed zeros.  The listen address is taken to be the path to a Unix domain
socket if -fcgi is given and unix domain sockets are supported (i.e. not
Windows).

Options:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	log.SetOutput(os.Stdout)

	/* Work out bomblet size */
	bs, err := humanize.ParseBytes(*bombletSize)
	if nil != err {
		log.Fatalf(
			"Unable to parse bomblet size %v: %v",
			*bombletSize,
			err,
		)
	}
	log.Printf("Compressing %v zeros", bs)

	/* Generate compressed zeros */
	BOMBLET, err = compress(bs)
	if nil != err {
		log.Fatalf("Unable to compress bytes: %v", err)
	}
	log.Printf(
		"Bomblet length %v bytes (~%v, compression ratio %.2f:1)",
		len(BOMBLET),
		humanize.Bytes(uint64(len(BOMBLET))),
		float64(bs)/float64(len(BOMBLET)),
	)

	/* Register handler */
	http.HandleFunc("/", handler)

	/* Serve, as appropriate */
	if *doFCGI {
		p, err := parsePerm(*sockPerm)
		if nil != err {
			log.Fatalf(
				"Unable to parse permissions %v: %v",
				*sockPerm,
				err,
			)
		}
		err = serveFCGI(*laddr, p)
	} else if *doTLS {
		log.Printf("Listening on %v for HTTPS requests", *laddr)
		err = http.ListenAndServeTLS(*laddr, *certf, *keyf, nil)
	} else {
		log.Printf("Listening on %v for HTTP requests", *laddr)
		err = http.ListenAndServe(*laddr, nil)
	}

	if nil != err {
		log.Fatalf("Serve error: %v", err)
	} else {
		log.Printf("Done.")
	}
}

/* serveFCGI serves up bomblets via FCGI.  It assumes laddr is a path to a
unix socket (on unix). */
func serveFCGI(laddr string, perm os.FileMode) error {
	var (
		l   net.Listener
		err error
	)

	/* Windows doesn't do unix sockets, of course */
	if "windows" == runtime.GOOS {
		l, err = net.Listen("tcp", laddr)
	} else {
		l, err = listenUnix(laddr, perm)
		/* Trap sigint to remove socket */
		ch := make(chan os.Signal)
		signal.Notify(ch, os.Interrupt)
		go func() {
			<-ch
			signal.Stop(ch)
			log.Printf(
				"Caught interrupt, removing %v",
				l.Addr().String(),
			)
			l.Close()
		}()
	}
	if nil != err {
		log.Fatalf("Unable to listen on %v: %v", laddr, err)
	}
	defer l.Close()
	log.Printf("Serving FCGI requests on %v", l.Addr())

	/* Serve FCGI requests */
	return fcgi.Serve(l, nil)
}

/* handler sends the bomblets out */
func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	tot := 0
	for {
		n, err := w.Write(BOMBLET)
		tot += n
		if nil != err {
			break
		}
	}
	log.Printf(
		"[%v] %q %v %q %v",
		r.RemoteAddr,
		r.Host,
		r.Method,
		r.URL,
		tot,
	)
}

// COMPRESSCHUNK is how many zeros to compress at once
const COMPRESSCHUNK = 1024

/* Compress compresses n bytes, 1k at a time */
func compress(n uint64) ([]byte, error) {
	/* Output buffer */
	buf := &bytes.Buffer{}
	/* Compressor, which stores to buf */
	zw, err := gzip.NewWriterLevel(buf, gzip.BestCompression)
	if nil != err {
		return nil, err
	}
	/* Write zeros to the compressor */
	var (
		zeros = make([]byte, COMPRESSCHUNK) /* Zeros to compress */
		tot   = uint64(0)                   /* Number written */
	)
	for tot < n {
		/* For the last chunk, only write as many zeros as we need */
		if uint64(len(zeros)) > n-tot {
			zeros = zeros[:n-tot]
		}
		/* Write zeros to the compressor */
		nw, err := zw.Write(zeros)
		if nil != err {
			return nil, err
		}
		tot += uint64(nw)
	}
	if err := zw.Close(); nil != err {
		return nil, err
	}
	return buf.Bytes(), nil
}

/* listenUnix makes a listening unix socket with path p, removing an existing
socket if one exists.  It sets the mode to mode and the UnlinkOnClose flag to
true. */
func listenUnix(u string, mode os.FileMode) (net.Listener, error) {
	/* Listen on a unix socket */
	ua, err := net.ResolveUnixAddr("unix", u)
	if nil != err {
		return nil, err
	}
	/* Remove sokcet if it exists */
	os.Remove(u)
	/* Make listener */
	ul, err := net.ListenUnix("unix", ua)
	if nil != err {
		return nil, err
	}
	/* Set perms and flag */
	ul.SetUnlinkOnClose(true)
	if err = os.Chmod(ul.Addr().String(), mode); nil != err {
		ul.Close()
		return nil, err
	}

	return ul, nil
}

/* parsePerm turns a numerical string permission into an os.FileMode.  It only
considers the lower nine bits. */
func parsePerm(p string) (os.FileMode, error) {
	/* Make sure it's a number */
	i, err := strconv.ParseUint(p, 8, 32)
	if nil != err {
		return 0, err
	}
	/* Too large no worky */
	if 0777 < i {
		return 0, fmt.Errorf("invalid octal permissions")
	}
	/* Mask off the lower nine bits */
	return os.FileMode(i & 0x000001FF), nil
}
