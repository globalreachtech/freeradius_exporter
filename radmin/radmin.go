package radmin

import (
	"log"
	"sync"
	"regexp"
	"strings"
	"os/exec"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var cmdHeadRegex = regexp.MustCompile(`^stats home_server (.+) ([0-9]+)$`)
var elapsedRegex = regexp.MustCompile(`^elapsed\.(.+)\t(\d+)$`)

// RAdminCollector type.
type RAdminCollector struct {
	mutex sync.Mutex
	cmdArgs []string
	hasServers bool
}

// NewRAdminCollector creates an RAdminCollector.
func NewRAdminCollector(socketFile string, homeServers []string) *RAdminCollector {
	// Build radmin command args
	args := []string{"-f", socketFile, "-E"}
	hasServers := false

	for _, hs := range homeServers {
		if hs == "" {
			continue
		}
		hsParts := strings.Split(hs, ":")
		args = append(args, "-e", "stats home_server " + hsParts[0] + " " + hsParts[1])
		hasServers = true
	}

	return &RAdminCollector{
		cmdArgs: args,
		hasServers: hasServers,
	}
}

// Describe outputs metrics descriptions.
func (r *RAdminCollector) Describe(ch chan<- *prometheus.Desc) {
	// nothing
}

// Collect fetches metrics from and sends them to the provided channel.
func (r *RAdminCollector) Collect(ch chan<- prometheus.Metric) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.hasServers {
		return
	}

	// Execute radmin to collect stats from freeradius
	radminOutput, cmdErr := exec.Command("/opt/sbin/radmin", r.cmdArgs...).CombinedOutput()
	if cmdErr != nil {
        log.Println("radmin error:", cmdErr)
		log.Println(string(radminOutput))
		return
    }
	
	currHs := "127.0.0.1:1812"
	outLines := strings.Split(string(radminOutput), "\n")
	for _, line := range outLines {
		// Check if line is the start of a new command result
		headMatches := cmdHeadRegex.FindStringSubmatch(line)
		if len(headMatches) == 3 {
			// Set homeserver address for subsequent parsed lines
			currHs = headMatches[1] + ":" + headMatches[2]
			continue
		}
		
		// Check if line is an elapsed stat
		elapsedMatches := elapsedRegex.FindStringSubmatch(line)
		if len(elapsedMatches) == 3 {
			// Parse latency range and request count
			i, err := strconv.ParseFloat(elapsedMatches[2], 64)
			if err != nil {
    			continue;
			}
			eName := elapsedMatches[1]
			
			// Add to prometheus
			ch <- prometheus.MustNewConstMetric(prometheus.NewDesc("freeradius_latency_"+eName, "Total requests taking over "+eName, []string{"address"}, nil), prometheus.CounterValue, i, currHs)
		}
	}
}
