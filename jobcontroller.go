package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	proxy "github.com/btbd/proxy/client"
)

var restartTimeout = 30 * time.Second

type Service struct {
	Index    int
	Start    time.Time
	End      time.Time
	Status   string // PASS, FAIL, RETRIES, ERROR:...
	Attempts int    // Try number

	mutex sync.Mutex
	job   *Job
	ping  time.Time `json:"-"` // Non-zero == it's running
}

func (t *Service) Run(isRestart bool) {
	t.mutex.Lock()

	t.ping = time.Now()
	t.Start = t.ping
	t.Status = ""
	t.Attempts++
	if !isRestart {
		t.job.ServiceStarted()
	}

	t.mutex.Unlock()

	// TODO: just put curl into gofunc - not all of this
	go func() {
		log.Printf("Start service: %s/%d\n", t.job.ID, t.Index)

		job := t.job

		host := job.ServiceName
		path := ""
		query := ""

		// Remove and save query parameters
		parts := strings.SplitN(job.ServiceName, "?", 2)
		if len(parts) > 1 {
			host = parts[0]
			query = "?" + parts[1]
		}

		// Remove and save any "path" part of the URL
		parts = strings.SplitN(host, "/", 2)
		if len(parts) > 1 {
			host = parts[0]
			path = "/" + parts[1]
		}

		/*
			// Append the "async" query parameter
				if query != "" {
					query += "&async"
				} else {
					query += "?async"
				}
		*/

		url := fmt.Sprintf("http://%s.default.svc.cluster.local%s%s",
			host, path, query)
		req, err := http.NewRequest("GET", url, nil)
		req.Header.Add("K_JOB_NAME", job.Name)
		req.Header.Add("K_JOB_ID", job.ID)
		req.Header.Add("K_JOB_INDEX", strconv.Itoa(t.Index))
		req.Header.Add("K_JOB_SIZE", strconv.Itoa(job.NumJobs))
		req.Header.Add("K_JOB_ATTEMPT", strconv.Itoa(t.Attempts))

		// Use the async HTTP header
		req.Header.Add("Prefer", "respond-async")

		for _, env := range job.Envs {
			req.Header.Add("K_ENV", env)
		}

		for i, arg := range job.Args {
			req.Header.Add("K_ARG_"+strconv.Itoa(i+1), arg)
		}

		log.Printf("Calling:%s\n", url)
		res, err := (&http.Client{}).Do(req)
		// res, err := Proxy.Do(&http.Client{}, req)

		body := ""
		sc := 0
		if res != nil && res.Body != nil {
			var buf = []byte{}
			buf, _ = ioutil.ReadAll(res.Body)
			body = string(buf)
			res.Body.Close()
		}
		if res != nil {
			sc = res.StatusCode
		}
		log.Printf("curl res(%d): %s\n", sc, body)

		if err != nil {
			t.Fail("Error talking to service: " + err.Error())
		} else if sc/100 != 2 {
			str := res.Status
			if body != "" {
				str += " - " + body
			}
			t.Fail("Error talking to service: " + str)
		}
	}()
}

func (t *Service) IsRunning() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return !t.ping.IsZero()
}

func (t *Service) TouchPing() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Already done running, ignore rogue/late messages
	if !t.End.IsZero() {
		return
	}

	t.ping = time.Now()
}

func (t *Service) Pass() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Already done running, ignore rogue/late messages
	if !t.End.IsZero() {
		return
	}

	t.ping = time.Now()
	t.End = t.ping
	t.Status = "PASS"

	t.job.ServiceEnded(true)
}

func (t *Service) Fail(reason string) {
	t.mutex.Lock()
	// Can't defer the unlock due to the t.Run() below

	// Already done running, ignore rogue/late messages
	if !t.End.IsZero() {
		t.mutex.Unlock()
		return
	}

	if t.job.MaxRetries == -1 || t.Attempts <= t.job.MaxRetries {
		t.mutex.Unlock()
		t.Run(true)
		return
	}

	// All retries have failed so give up
	t.ping = time.Now()
	t.End = t.ping
	t.Status = "FAIL: " + reason

	t.mutex.Unlock()

	t.job.ServiceEnded(false)
}

type Job struct {
	ID     string
	Name   string
	Status string // PENDING, RUNNING, PASS, FAIL, DELETING

	DependsOn   string `json:",omitempty"` // JobID
	DependsWhen string `json:",omitempty"` // PASS, FAIL, ANY
	DependsType string `json:",omitempty"` // ALL, INDEX

	ServiceName string
	NumJobs     int
	Parallel    int
	MaxRetries  int
	Envs        []string `json:",omitempty"`
	Args        []string `json:",omitempty"`

	mutex        sync.Mutex
	Start        time.Time
	End          time.Time
	NumRunning   int
	NumCompleted int
	NumPassed    int
	Services     []*Service
}

var Jobs = map[string]*Job{}     // ID->*Job
var Name2Job = map[string]*Job{} // JobName->*Job

var JobsMutex = sync.Mutex{}

func AddJob(job *Job) {
	JobsMutex.Lock()
	defer JobsMutex.Unlock()

	Jobs[job.ID] = job
	Name2Job[job.Name] = job
}

func DelJob(job *Job) {
	JobsMutex.Lock()
	defer JobsMutex.Unlock()

	delete(Jobs, job.ID)
	delete(Name2Job, job.Name)
}

func GetJobByID(id string) *Job {
	JobsMutex.Lock()
	defer JobsMutex.Unlock()

	return Jobs[id]
}

func GetJobByName(name string) *Job {
	JobsMutex.Lock()
	defer JobsMutex.Unlock()

	return Name2Job[name]
}

func (j *Job) ServiceStarted() {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.NumRunning++
}

func (j *Job) ServiceEnded(pass bool) {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	j.NumCompleted++
	j.NumRunning--
	if pass {
		j.NumPassed++
	}

	if j.NumCompleted == j.NumJobs {
		j.End = time.Now()

		if j.NumPassed == j.NumJobs {
			j.Status = "PASS"
		} else {
			j.Status = "FAIL"
		}
	}
}

func (j *Job) MarkDelete() {
	j.Status = "DELETING"
}

func (j *Job) Delete() {
	DelJob(j)
}

func Controller() {
	for {
		for _, job := range Jobs {
			now := time.Now()

			if job.NumCompleted == job.NumJobs {
				// All done so delete it
				// job.MarkDelete()
			}

			if job.Status == "DELETING" {
				// TODO: check for dependencies
				job.Delete()
				continue
			}

			if job.Status == "PENDING" {
				job.Status = "RUNNING"
				job.Start = now
			}

			// Look for timed-out jobs
			for _, service := range job.Services {
				// Skip jobs that are not running
				if service.ping.IsZero() {
					continue
				}

				if now.Sub(service.ping) > restartTimeout {
					service.Fail("Ping timeout")
				}
			}

			// If we have room, find one to run
			if job.NumCompleted != job.NumJobs && job.NumRunning < job.Parallel {
				for _, service := range job.Services {
					// Skip jobs that are done or running
					if !service.ping.IsZero() || !service.End.IsZero() {
						continue
					}

					service.Run(false)

					// Exit if we're at max concurrency/parallel
					if job.NumRunning == job.Parallel {
						break
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
		// time.Sleep(5 * time.Second)
	}
}

func curl(url string) (string, error) {
	res, err := http.Get(url)
	body := ""
	if res != nil && res.Body != nil {
		var buf = []byte{}
		buf, _ = ioutil.ReadAll(res.Body)
		body = string(buf)
		res.Body.Close()
	}
	return body, err
}

var Proxy *proxy.Proxy

const ProxyURL = "http://proxy.default.svc.cluster.local"

func SetupProxy() {
	var err error
	Proxy, err = proxy.NewWithConfig(ProxyURL, proxy.Config{
		NumberOfSenders: 1,
		DebugLevel:      1,
		DebugPrint:      log.Printf,
	})
	if err != nil {
		panic(err)
	}
}

func main() {
	log.SetFlags(log.Ltime)
	go Controller()
	go SetupProxy()

	http.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		serviceName := r.URL.Query().Get("service")
		dependsOn := r.URL.Query().Get("dependson")
		dependsWhen := "PASS"
		numJobs := r.URL.Query().Get("num")
		parallel := r.URL.Query().Get("parallel")
		retry := r.URL.Query().Get("retry")
		envs := r.URL.Query()["env"]

		var err error
		id := fmt.Sprintf("%d", time.Now().UnixNano()) // fix

		if jobName == "" {
			jobName = id
		}

		// If job by this name already exists then it's an error
		if job := GetJobByName(jobName); job != nil {
			// For now if it already exists then delete it and create
			// a new one with the same name, if it's done! If it's running
			// then stop them from killing it by returning an error
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Job '" + jobName + "' already exists\n"))
			return
		}

		args := map[int]string{}
		for name, values := range r.Header {
			name = strings.ToUpper(name)
			if strings.HasPrefix(name, "ARG_") {
				i, err := strconv.Atoi(name[4:])
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("Bad arg '" + name + "'\n"))
				}
				args[i] = values[0]
			}
		}

		if serviceName == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing 'service'\n"))
			return
		}

		if dependsOn != "" {
			// JOB[:pass|fail]
			parts := strings.SplitN(dependsOn, ":", 2)
			when := ""

			parts[0] = strings.TrimSpace(parts[0])
			if parts[0] == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Missing 'job' on 'dependson'\n"))
				return
			}

			job := GetJobByID(parts[0])

			if job == nil {
				job = GetJobByName(parts[0])
				if job == nil || job.Status == "DELETING" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("Can't find job '" + parts[0] + "'\n"))
					return
				}
			}

			if len(parts) > 1 {
				when = strings.ToUpper(strings.TrimSpace(parts[1]))
				if when != "" && when != "PASS" && when != "FAIL" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte("Invalid dependency type '" + when + "'\n"))
					return
				}
				if when == "" {
					when = "PASS"
				}
			}
			dependsOn = job.ID
			dependsWhen = when
		}

		job := Job{
			Name:   jobName,
			Status: "PENDING",

			DependsOn:   dependsOn,
			DependsWhen: dependsWhen,

			ServiceName: serviceName,
			NumJobs:     1,
			Parallel:    10,
			MaxRetries:  0,
			Envs:        envs,

			ID: id,
			// Start: time.Now(),
		}

		if numJobs != "" {
			if job.NumJobs, err = strconv.Atoi(numJobs); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad 'num': " + err.Error() + "\n"))
				return
			}
			if job.NumJobs <= 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid 'num' value: " + numJobs + "\n"))
				return
			}
		}

		if parallel != "" {
			if job.Parallel, err = strconv.Atoi(parallel); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad 'parallel'\n" + err.Error()))
				return
			}
			if job.Parallel < 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid 'parallel' value: " + parallel + "\n"))
				return
			}
		}

		if retry != "" {
			if job.MaxRetries, err = strconv.Atoi(retry); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad 'retry'\n" + err.Error()))
				return
			}

			if job.MaxRetries < -1 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid 'retry' value: " + numJobs + "\n"))
				return
			}
		}

		job.Services = make([]*Service, job.NumJobs)
		for i := range job.Services {
			job.Services[i] = &Service{
				Index: i,
				job:   &job,
			}
		}

		for i := 1; ; i++ {
			value, ok := args[i]
			if !ok {
				break
			}
			job.Args = append(job.Args, value)
		}

		AddJob(&job)

		w.Header().Add("JOB-NAME", job.Name)
		w.Header().Add("JOB-ID", job.ID)
	})

	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		buf := []byte{}

		if jobName != "" {
			job := GetJobByName(jobName)
			if job == nil {
				job = GetJobByID(jobName)
			}
			if job == nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Can't find '" + jobName + "'\n"))
				return
			}
			buf, _ = json.MarshalIndent(job, "", "  ")
		} else {
			jobs := []*Job{}
			for _, job := range Jobs {
				jobs = append(jobs, job)
			}
			buf, _ = json.MarshalIndent(jobs, "", "  ")
		}

		w.Write(buf)
		w.Write([]byte("\n"))
	})

	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		jobID := r.URL.Query().Get("job")
		serviceIndex := r.URL.Query().Get("index")
		serviceStatus := r.URL.Query().Get("status")

		job := GetJobByID(jobID)
		if job == nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Can't find job '" + jobID + "'\n"))
			return
		}

		index, err := strconv.Atoi(serviceIndex)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Can't parse:" + serviceIndex + ":" + err.Error() + "\n"))
			return
		}

		if index > len(job.Services) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Index '" + serviceIndex + "' is too big\n"))
			return
		}

		if serviceStatus == "pass" {
			log.Printf("Got PASS %s %d\n", jobID, index)
			job.Services[index].Pass()
		} else if serviceStatus == "fail" {
			log.Printf("Got FAIL %s %d\n", jobID, index)
			job.Services[index].Fail("Execution failed")
		} else {
			log.Printf("Got PING %s %d\n", jobID, index)
			job.Services[index].TouchPing()
		}
	})

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		jobNames := r.URL.Query().Get("jobs")
		_, ok := r.URL.Query()["all"]

		if ok {
			if jobNames != "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Can't specify 'all' and a job\n"))
				return
			}
			for _, job := range Jobs {
				job.MarkDelete()
			}
			return
		}

		jobs := []*Job{}
		for _, jobName := range strings.Split(jobNames, ",") {
			job := GetJobByName(jobName)
			if job == nil {
				job = GetJobByID(jobName)
			}
			if job == nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Can't find job '" + jobName + "'\n"))
				return
			}
			jobs = append(jobs, job)
		}
		for _, job := range jobs {
			job.MarkDelete()
		}
	})

	log.Print("Job controller listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}
