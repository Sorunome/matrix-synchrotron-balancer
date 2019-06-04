package main

import (
	"net"
	"log"
	"regexp"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"matrix-synchrotron-balancer/config"
	"bytes"
	"strconv"
	"time"
	"github.com/struCoder/pidusage"
)

type Synchrotron struct {
	Address string
	PIDFile string
	Load float64
	Users int
	RelocateCounter float64
}

var rMatchToken *regexp.Regexp
var tokenMxidCache map[string]string
var synchrotronCache map[string]int
var numSynchrotrons = 0
var synchrotrons = []*Synchrotron{}
var totalUsers = 0

func getSynchrotron(mxid string) int {
	if val, ok := synchrotronCache[mxid]; ok {
		if synchrotrons[val].RelocateCounter < config.Get().Balancer.RelocateCounterThreshold {
			return val
		}
		// we need to relocate the user to another synchrotron
		synchrotrons[val].Users--
		// estimate to how good our relocating is
		synchrotrons[val].RelocateCounter -=  config.Get().Balancer.RelocateCooldown
	}
	minLoad := 1000.0
	i := 0
	for ii, synch := range synchrotrons {
		if synch.Load < minLoad {
			minLoad = synch.Load
			i = ii
		}
	}
	synchrotronCache[mxid] = i
	synchrotrons[i].Users++
	return i
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
	
	rconn, err = net.Dial("tcp", synchrotrons[synchIndex].Address)
	if err != nil {
		log.Print("Failed to connect to remote")
		return
	}
	
	// don't forget to send the first chunk!
	_, err = rconn.Write(b)
	if err != nil {
		return
	}
	
	totalUsers++
	
	var pipe = func(src net.Conn, dst net.Conn) {
		defer func() {
			totalUsers--
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

func updateLoads() {
	minLoad := 1000.0
	maxLoad := 0.0
	for i, synch := range synchrotrons {
		f, err := ioutil.ReadFile(synch.PIDFile)
		if err != nil {
			log.Print("Error fetching PID file: ", err)
			continue
		}
		pid, err := strconv.Atoi(string(bytes.TrimSpace(f)))
		if err != nil {
			log.Print("Malformed PID file: ", err)
			continue
		}
		sysInfo, err := pidusage.GetStat(pid)
		if err != nil {
			log.Print("Error fetching synchrotron stats: ", err)
		}
		synch.Load = sysInfo.CPU
		if synch.Load > maxLoad {
			maxLoad = synch.Load
		}
		if synch.Load < minLoad {
			minLoad = synch.Load
		}
		log.Print("Synchrotron ", i, " Users:", synch.Users, " Load:", synch.Load)
	}
	relocateLoad := minLoad * config.Get().Balancer.RelocateThreshold
	for _, synch := range synchrotrons {
		if synch.Load >= relocateLoad && synch.Users > 1 && synch.Load > config.Get().Balancer.RelocateMinCpu {
			synch.RelocateCounter++
		} else if synch.RelocateCounter > 0 {
			synch.RelocateCounter--
		}
		if synch.RelocateCounter < 0 {
			synch.RelocateCounter = 0
		}
	}
}

func startUpdateLoads() {
	for {
		time.Sleep(time.Duration(config.Get().Balancer.Interval) * time.Second)
		go updateLoads()
	}
}

func main() {
	var err error
	rMatchToken, err = regexp.Compile("(?i)(?:Authorization:\\s*\\S*\\s*(\\S*)|[&?]token=([^&?\\s]+))")
	if err != nil {
		log.Panic("Invalid regex", err)
	}
	tokenMxidCache = make(map[string]string)
	synchrotronCache = make(map[string]int)

	if config.Get().Synchrotrons == nil {
		log.Panic("Please configure at least one synchrotron")
	}

	for _, synch := range config.Get().Synchrotrons {
		synchrotrons = append(synchrotrons, &Synchrotron{
			Address: synch.Address,
			PIDFile: synch.PIDFile,
			Load: 0,
			Users: 0,
			RelocateCounter: 0,
		})
	}
	numSynchrotrons = len(synchrotrons)
	log.Print("Configured synchrotrons: ", numSynchrotrons)

	go startUpdateLoads()
	updateLoads()

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
