zipbombserver
=============
Serves up never-endng compressed zeros in an http response.  Useful for shooing
away some crude bots and scanners.  And nmap.

This is useful for filling the memory buffer or disk space of a scanner with
useless information, while keeping transmission sizes at about 1/1000th of the
decompressed size.

Inspired by
https://www.hackerfactor.com/blog/index.php?/archives/762-Attacked-Over-Tor.html

Not for illegal use.

Install
-------
```bash
go get -u github.com/magisterquis/zipbombserver
```

Running
-------
Zipbombserver can either be used to listen for HTTP requests (possibly behind a
TLS-terminating reverse proxy), or it can communicate with a webserver via
FastCGI.

### HTTP Server
The HTTP server serves up HTTP requests.  Aside from the body causing clients a
bit of trouble, it is a nearly totally unremarkable HTTP server.  HTTPS can be
served with the `-https` flag, using the `-cert` and `-key` flags.

### FastCGI
To better integrate with existing setups, zipbombserver can serve FastCGI
requests over a Unix domain socket (or TCP socket, on Windows), settable with
the `-l` option.  FastCGI is enabled with the `-fcgi` flag.

If a socket with the same path already exists (on Unix), the existing socket
will be removed before a new socket is created.  The created socket will be
removed before termination if zipbombserver receives a SIGINT.

Bomblets
--------
Gzip is used to compress zeros at around a 1000:1 ratio.  Because gzip is used
multiple gzipped blocks of data can be concatenated and will be decompressed as
one stream.  The practical upshot of this is that only a small number of zeros
(a bomblet) need to be compressed, and can be sent over and over in the body of
the http response.  It may be worth playing around with different sizes for IDS
evasion.  In practice, 10MB of pre-compressed zeros (the default) seems to work
pretty well.

Logging
-------
Log messages are written to the standard output, and consist of lines of the
form 

```
timestamp [remote address] "Host" METHOD "/path" 
```
As an example,
```
2017/05/24 23:13:45 [192.168.111.222:30793] "noproblems.ccc" GET "/somepath" 7607340
```
indicates that shortly before midnight, `192.168.111.222` performed a `GET`
request for `/somepath` to the host `noproblems.ccc` (read from the `Host`
header), and 7607340 were attepted to be sent (though many may have been
buffered and never made it to the wire).
