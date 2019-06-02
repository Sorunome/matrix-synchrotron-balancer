package main

import (
	"net"
	"log"
	"regexp"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"matrix-synchrotron-balancer/config"
)

type Synchrotron struct {
	Url string
	PIDFile string
	Load int
}

var rMatchToken *regexp.Regexp
var tokenMxidCache map[string]string
var synchrotronCache map[string]int
var numSynchrotrons = 0
var synchrotrons = []*Synchrotron{}

func getSynchrotron(mxid string) int {
	if val, ok := synchrotronCache[mxid]; ok {
		return val
	}
	minLoad := 99999999
	synchIndex := 0
	for i, synch := range synchrotrons {
		if synch.Load < minLoad {
			minLoad = synch.Load
			synchIndex = i
		}
	}
	synchrotronCache[mxid] = synchIndex
	return synchIndex
}

func getMxid(token []byte) string {
	sToken := string(token)
	if val, ok := tokenMxidCache[sToken]; ok {
		return val
	}
	log.Print("New first authorization token")
	req, err := http.NewRequest("GET", config.Get().HomeserverUrl + "/_matrix/client/r0/account/whoami", nil)
	req.Header.Add("Authorization", "Bearer " + sToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Print("Error fetching user id from home server: ", err)
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
	userId := m.UserId
	log.Print("New user id: ", userId)
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
	synchIndex := getSynchrotron(mxid)
	
	rconn, err = net.Dial("tcp", synchrotrons[synchIndex].Url)
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
	tokenMxidCache = make(map[string]string)
	synchrotronCache = make(map[string]int)

	for _, synch := range config.Get().Synchrotrons {
		synchrotrons = append(synchrotrons, &Synchrotron{
			Url: synch.Url,
			PIDFile: synch.PIDFile,
			Load: 0,
		})
	}
	numSynchrotrons = len(synchrotrons)
	log.Print("Configured synchrotrons: ", numSynchrotrons)

	ln, err := net.Listen("tcp", config.Get().Listener)
	if err != nil {
		log.Panic("Error starting up:", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Print("Error accepting connection")
			continue
		}
		go handleConnection(conn)
	}
}
