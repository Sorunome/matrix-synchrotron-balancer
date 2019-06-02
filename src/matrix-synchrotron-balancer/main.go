package main

import (
	"net"
	"log"
	"regexp"
	"net/http"
	"io/ioutil"
	"encoding/json"
)

var rMatchToken *regexp.Regexp
var rMatchUserId *regexp.Regexp
var tokenMxidCache map[string]string

func getMxid(token []byte) string {
	sToken := string(token)
	if val, ok := tokenMxidCache[sToken]; ok {
		log.Print("Using cache")
		return val
	}
	req, err := http.NewRequest("GET", "http://localhost:8008/_matrix/client/r0/account/whoami", nil)
	req.Header.Add("Authorization", "Bearer " + sToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		tokenMxidCache[sToken] = ""
		return ""
	}
	body, _ := ioutil.ReadAll(resp.Body)
	var m struct {
		UserId string `json:"user_id"`
	}
	err = json.Unmarshal(body, &m);
	if err != nil {
		log.Print("JSON decode error: ", err)
		tokenMxidCache[sToken] = ""
		return ""
	}
	log.Print(m)
	userId := m.UserId
	tokenMxidCache[sToken] = userId
	return userId
}

func handleConnection(conn net.Conn) {
	var rconn net.Conn
	var err error
	
	// read out the first chunk to determine where to route to
	buff := make([]byte, 65535)
	n, err := conn.Read(buff)
	if err != nil {
		return
	}
	b := buff[:n]
	match := rMatchToken.FindSubmatch(b)
	mxid := ""
	if len(match) > 1 {
		token := match[1]
		if len(token) == 0 {
			token = match[2]
		}
		mxid = getMxid(token)
	}
	log.Print(mxid)
	
	rconn, err = net.Dial("tcp", "localhost:8008")
	if err != nil {
		log.Print("Failed to connect to remote")
		return
	}
	
	// don't forget to send the first chunk!
	_, err = rconn.Write(b)
	if err != nil {
		return
	}
	
	var pipe = func(src net.Conn, dst net.Conn) {
		defer func() {
			conn.Close()
			rconn.Close()
		}()
		buff := make([]byte, 65535)
		for {
			n, err := src.Read(buff)
			if err != nil {
				return
			}
			b := buff[:n]
			_, err = dst.Write(b)
			if err != nil {
				return
			}
		}
	}
	go pipe(conn, rconn)
	go pipe(rconn, conn)
}

func main() {
	var err error
	rMatchToken, err = regexp.Compile("(?i)(?:Authorization:\\s*\\S*\\s*(\\S*)|[&?]token=([^&?\\s]+))")
	if err != nil {
		log.Panic("Invalid regex", err)
	}
	rMatchUserId, err = regexp.Compile("(?i)user_id\\s*=\\s*(@[^:]+:\\S+)")
	if err != nil {
		log.Panic("Invalid regex", err)
	}
	tokenMxidCache = make(map[string]string)

	ln, err := net.Listen("tcp", "localhost:8083")
	if err != nil {
		log.Panic("Error starting up:", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Print("Error accepting connection")
			continue
		}
		log.Print("accepting new connection")
		go handleConnection(conn)
	}
}
