package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

var errorTemplate *template.Template = template.Must(template.New("error").Parse(`
	<html>
		<head>
			<title>Mist Error</title>
		</head>
		<body>
			<h1>Error while proxying request.</h1>
			<p>{{.}}</p>
		</body>
	</html>	
`))

// This timeout is the same as Apache's version > 2.2 timeout.
var HttpReadTimeout = 5 * time.Second

type HostProxy struct {
	mappings map[string]string
}

func NewHostProxy() *HostProxy {
	return &HostProxy{
		mappings: make(map[string]string),
	}
}

func valueMatchHostPattern(val, pattern string) bool {
	val = strings.ToLower(val)
	pattern = strings.ToLower(pattern)
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(val, pattern[1:]) ||
			val == pattern[2:]
	}
	return val == pattern
}

func (h *HostProxy) AddMapping(pattern, address string) {
	h.mappings[pattern] = address
}

func (h *HostProxy) findForwardAddressForHost(host string) string {
	for pattern, address := range h.mappings {
		if valueMatchHostPattern(host, pattern) {
			return address
		}
	}
	return ""
}

func getErrorResponse(req *http.Request, status string, code int, data interface{}) *http.Response {

	var buf bytes.Buffer
	errorTemplate.Execute(&buf, data)

	proto := "HTTP/1.0"
	protoMajor := 1
	protoMinor := 0

	if req != nil {
		proto = req.Proto
		protoMajor = req.ProtoMajor
		protoMinor = req.ProtoMinor
	}

	return &http.Response{
		Status:        status,
		StatusCode:    code,
		Proto:         proto,
		ProtoMajor:    protoMajor,
		ProtoMinor:    protoMinor,
		Body:          ioutil.NopCloser(&buf),
		ContentLength: int64(buf.Len()),
	}
}

func IsMalformedRequestError(err error) bool {
	return err == http.ErrContentLength ||
		err == http.ErrHeaderTooLong ||
		err == http.ErrShortBody ||
		err == http.ErrUnexpectedTrailer ||
		err == http.ErrMissingContentLength ||
		err == http.ErrNotMultipart ||
		err == http.ErrMissingBoundary
}

func (h *HostProxy) connectionHandler(conn net.Conn) {
	log.Print("Handling Connection")
	defer log.Print("Finished handling connection")
	defer conn.Close()

	// Setup the server connection
	serverConn := httputil.NewServerConn(conn, nil)
	defer serverConn.Close()

	requestClose := false

	// Get First Request
	conn.SetReadDeadline(time.Now().Add(HttpReadTimeout))
	req, err := serverConn.Read()
	if err == httputil.ErrPersistEOF {
		return
	} else if IsMalformedRequestError(err) {
		serverConn.Write(req, getErrorResponse(req, "400 Bad Request", http.StatusBadRequest, err))
		return
	} else if err != nil {
		serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", http.StatusServiceUnavailable, err))
		return
	}

	// Figure out the address
	address := h.findForwardAddressForHost(req.Host)
	if address == "" {
		serverConn.Write(req, getErrorResponse(req, "404 Not Found", http.StatusNotFound, "Not Found"))
	}

	// Open up the client connection to the proxy endpoint
	cTcp, err := net.Dial("tcp", address)
	if err != nil {
		serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", http.StatusServiceUnavailable, err))
		return
	}
	defer cTcp.Close()

	clientConn := httputil.NewClientConn(cTcp, nil)

	for {
		cTcp.SetReadDeadline(time.Now().Add(HttpReadTimeout))
		resp, err := clientConn.Do(req)
		if err == httputil.ErrPersistEOF {
			requestClose = true
			if resp == nil {
				return
			}
		} else if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", http.StatusServiceUnavailable, err))
			return
		}

		resp.Close = requestClose

		err = serverConn.Write(req, resp)
		if err != nil {
			// not sure what to do with an error here, besides just quitting
			return
		}

		if requestClose {
			break
		}

		// Read next request
		conn.SetReadDeadline(time.Now().Add(HttpReadTimeout))
		req, err = serverConn.Read()
		if err == httputil.ErrPersistEOF {
			// It's already determined that we have ended, so we may as well end
			return
		} else if IsMalformedRequestError(err) {
			serverConn.Write(req, getErrorResponse(req, "400 Bad Request", http.StatusBadRequest, err))
			return
		} else if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", http.StatusServiceUnavailable, err))
			return
		}
	}

}

func (h *HostProxy) LoadMappingsFrom(configFile string) error {
	f, err := os.Open(configFile)
	if err != nil {
		return err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	entries := make(map[string]string)
	d.Decode(&entries)

	for pattern, address := range entries {
		h.AddMapping(pattern, address)
	}

	return nil
}

func (h *HostProxy) ListenAndServe(address string) error {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go h.connectionHandler(conn)
	}

}
