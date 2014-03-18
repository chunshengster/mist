package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
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

	return &http.Response{
		Status:        status,
		StatusCode:    200,
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Body:          ioutil.NopCloser(&buf),
		ContentLength: -1,
	}
}

func (h *HostProxy) connectionHandler(serverConn *httputil.ServerConn) {
	defer serverConn.Close()
	for {
		req, err := serverConn.Read()
		if err != nil {
			return
		}

		address := h.findForwardAddressForHost(req.Host)
		if address == "" {
			serverConn.Write(req, getErrorResponse(req, "404 Not Found", 404, "Not Found"))
			return
		}

		conn, err := net.Dial("tcp", address)
		if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", 503, err))
			return
		}
		defer conn.Close()

		clientConn := httputil.NewClientConn(conn, nil)
		err = clientConn.Write(req)
		if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", 503, err))
			return
		}

		resp, err := clientConn.Read(req)
		if resp != nil {
			serverConn.Write(req, resp)
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
		go h.connectionHandler(httputil.NewServerConn(conn, nil))
	}

}
