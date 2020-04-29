package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/struCoder/pidusage"

	"github.com/Sorunome/matrix-synchrotron-balancer/config"
)

type Synchrotron struct {
	Address         string
	PIDFile         string
	Load            float64
	Users           int
	RelocateCounter float64
}

var rMatchToken = regexp.MustCompile(`(?i)(?:Authorization:\s*Bearer\s*(\S*)|[&?]access_token=([^&?\s]+))`)
var tokenMxidCache = make(map[string]string)
var synchrotronCache = make(map[string]int)
var synchrotrons []*Synchrotron
var totalUsers = 0

func getSynchrotron(mxid string) *Synchrotron {
	if cachedIndex, ok := synchrotronCache[mxid]; ok {
		if synchrotrons[cachedIndex].RelocateCounter < config.Get().Balancer.RelocateCounterThreshold || synchrotrons[cachedIndex].Users < 2 {
			return synchrotrons[cachedIndex]
		}
		// we need to relocate the user to another synchrotron
		synchrotrons[cachedIndex].Users--
		// estimate to how good our relocating is
		synchrotrons[cachedIndex].RelocateCounter -= config.Get().Balancer.RelocateCooldown
	}
	minLoad := 1000.0
	chosenIndex := 0
	for i, synch := range synchrotrons {
		if synch.Load < minLoad {
			minLoad = synch.Load
			chosenIndex = i
		}
	}
	synchrotronCache[mxid] = chosenIndex
	synchrotrons[chosenIndex].Users++
	return synchrotrons[chosenIndex]
}

type WhoamiResponse struct {
	UserID string `json:"user_id"`
}

func getMXID(token string) string {
	if val, ok := tokenMxidCache[token]; ok {
		return val
	}
	log.Println("New first authorization token")
	req, err := http.NewRequest("GET", config.Get().HomeserverURL+"/_matrix/client/r0/account/whoami", nil)
	if err != nil {
		log.Println("Error creating request to fetch user ID from homeserver:", err)
		return ""
	}
	req.Header.Add("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Error fetching user ID from homeserver:", err)
		tokenMxidCache[token] = ""
		return ""
	}
	defer resp.Body.Close()
	var whoami WhoamiResponse
	err = json.NewDecoder(resp.Body).Decode(&whoami)
	if err != nil {
		log.Println("JSON decode error:", err)
		tokenMxidCache[token] = ""
		return ""
	}
	log.Println("New user ID:", whoami.UserID)
	tokenMxidCache[token] = whoami.UserID
	return whoami.UserID
}

func pipe(src net.Conn, dst net.Conn, wg *sync.WaitGroup) {
	buff := make([]byte, 65535)
	_, _ = io.CopyBuffer(dst, src, buff)
	src.Close()
	dst.Close()
	wg.Done()
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	// read out the first chunk to determine where to route to
	buff := make([]byte, 65535)
	n, err := conn.Read(buff)
	if err != nil {
		return
	}

	firstChunk := buff[:n]
	var mxid string
	if match := rMatchToken.FindSubmatch(firstChunk); len(match) > 1 {
		token := match[1]
		if len(token) == 0 {
			token = match[2]
		}
		mxid = getMXID(string(token))
	}

	rconn, err := net.Dial("tcp", getSynchrotron(mxid).Address)
	if err != nil {
		log.Println("Failed to connect to remote")
		return
	}
	defer rconn.Close()

	// don't forget to send the first chunk!
	_, err = rconn.Write(firstChunk)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	totalUsers++

	go pipe(conn, rconn, &wg)
	go pipe(rconn, conn, &wg)

	wg.Wait()
	totalUsers--
}

func updateLoads() {
	minLoad := 1000.0
	maxLoad := 0.0
	for i, synch := range synchrotrons {
		f, err := ioutil.ReadFile(synch.PIDFile)
		if err != nil {
			log.Println("Error fetching PID file:", err)
			continue
		}
		pid, err := strconv.Atoi(string(bytes.TrimSpace(f)))
		if err != nil {
			log.Println("Malformed PID file:", err)
			continue
		}
		sysInfo, err := pidusage.GetStat(pid)
		if err != nil {
			log.Println("Error fetching synchrotron stats:", err)
		}
		synch.Load = sysInfo.CPU
		if synch.Load > maxLoad {
			maxLoad = synch.Load
		}
		if synch.Load < minLoad {
			minLoad = synch.Load
		}
		log.Println("Synchrotron", i, "Users:", synch.Users, "Load:", synch.Load)
	}
	relocateLoad := minLoad * config.Get().Balancer.RelocateThreshold
	for _, synch := range synchrotrons {
		if synch.Load >= relocateLoad && synch.Users > 1 && synch.Load > config.Get().Balancer.RelocateMinCPU {
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
	if len(config.Get().Synchrotrons) == 0 {
		log.Panic("Please configure at least one synchrotron")
	}

	synchrotrons = make([]*Synchrotron, len(config.Get().Synchrotrons))
	for i, synch := range config.Get().Synchrotrons {
		synchrotrons[i] = &Synchrotron{
			Address:         synch.Address,
			PIDFile:         synch.PIDFile,
			Load:            0,
			Users:           0,
			RelocateCounter: 0,
		}
	}
	log.Print("Configured synchrotrons: ", len(synchrotrons))

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
