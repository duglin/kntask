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
)

var restartTimeout = 10 * time.Second

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
		url := fmt.Sprintf("http://%s.default.svc.cluster.local?async",
			job.ServiceName)
		req, err := http.NewRequest("GET", url, nil)
		req.Header.Add("K_JOB_NAME", job.Name)
		req.Header.Add("K_JOB_ID", job.ID)
		req.Header.Add("K_JOB_INDEX", strconv.Itoa(t.Index))
		req.Header.Add("K_JOB_ATTEMPT", strconv.Itoa(t.Attempts))

		for _, env := range job.Envs {
			req.Header.Add("K_ENV", env)
		}

		for i, arg := range job.Args {
			req.Header.Add("K_ARG_"+strconv.Itoa(i+1), arg)
		}

		log.Printf("Calling:%s\n", url)
		res, err := (&http.Client{}).Do(req)

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
			log.Printf("Curl res: %s\n", body)
			t.Fail("Error talking to service: " + err.Error())
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

	if t.Attempts <= t.job.NumRetries {
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
	ID          string
	Name        string
	ServiceName string
	NumJobs     int
	Parallel    int
	NumRetries  int
	Flavor      string
	Envs        []string
	Args        []string

	mutex        sync.Mutex
	Start        time.Time
	End          time.Time
	NumRunning   int
	NumCompleted int
	NumPassed    int
	Services     []*Service
}

var Jobs = map[string]*Job{}         // ID->*Job
var Name2JobID = map[string]string{} // JobName->JobID

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
	}
}

func Controller() {
	for {
		for _, job := range Jobs {
			if job.NumCompleted == job.NumJobs {
				// All done so delete it
				// delete(Jobs, jID)
			}

			// Look for timed-out jobs
			now := time.Now()
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

func main() {
	log.SetFlags(log.Ltime)
	go Controller()

	http.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		serviceName := r.URL.Query().Get("service")
		numJobs := r.URL.Query().Get("num")
		parallel := r.URL.Query().Get("parallel")
		retry := r.URL.Query().Get("retry")
		flavor := r.URL.Query().Get("flavor")
		envs := r.URL.Query()["env"]

		var err error
		id := fmt.Sprintf("%d", time.Now().UnixNano()) // fix

		if jobName == "" {
			jobName = id
		}

		if jobID, ok := Name2JobID[jobName]; ok {
			// For now if it already exists then delete it and create
			// a new one with the same name, if it's done! If it's running
			// then stop them from killing it by returning an error
			job := Jobs[jobID]
			if job.NumCompleted != job.NumJobs {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Job '" + jobName + "' already exists and is running\n"))
				return
			}

			delete(Jobs, jobID)
			delete(Name2JobID, jobName)
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

		job := Job{
			Name:        jobName,
			ServiceName: serviceName,
			NumJobs:     1,
			Parallel:    10,
			NumRetries:  0,
			Flavor:      flavor,
			Envs:        envs,

			ID:    id,
			Start: time.Now(),
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
			if job.NumRetries, err = strconv.Atoi(retry); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad 'retry'\n" + err.Error()))
				return
			}

			if job.NumRetries < 0 {
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

		Jobs[job.ID] = &job
		Name2JobID[job.Name] = job.ID

		r.Header.Add("JOB-NAME", job.Name)
		r.Header.Add("JOB-ID", job.ID)
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		jobID := ""

		if jobName != "" {
			if jobID = Name2JobID[jobName]; jobID == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Can't find job '" + jobName + "'\n"))
				return
			}
		}

		buf := []byte{}
		if jobID == "" {
			buf, _ = json.MarshalIndent(Jobs, "", "  ")
		} else {
			job, ok := Jobs[jobID]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Can't find job '" + jobName + "'\n"))
				return
			}
			copyJob := *job

			buf, _ = json.MarshalIndent(&copyJob, "", "  ")
			if copyJob.NumCompleted == copyJob.NumJobs {
				delete(Jobs, jobID)
				delete(Name2JobID, jobName)
			}
		}
		w.Write(buf)
		w.Write([]byte("\n"))
	})

	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		jobID := r.URL.Query().Get("job")
		serviceIndex := r.URL.Query().Get("index")
		serviceStatus := r.URL.Query().Get("status")

		job, ok := Jobs[jobID]
		if !ok {
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

	log.Print("Job controller listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}
