package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

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

func getErrorResponse(req *http.Request, status string, code int) *http.Response {
	return &http.Response{
		Status:     status,
		StatusCode: code,
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
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
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", 503))
			return
		}

		conn, err := net.Dial("tcp", address)
		if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", 503))
			return
		}
		defer conn.Close()
		clientConn := httputil.NewClientConn(conn, nil)
		err = clientConn.Write(req)
		if err != nil {
			serverConn.Write(req, getErrorResponse(req, "503 Service Unavailable", 503))
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
