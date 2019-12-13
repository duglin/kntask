package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

var restartTimeout = 10 * time.Second

type Status struct {
	Ping     time.Time `json:"-"`
	Start    time.Time
	End      time.Time
	Status   string // PASS, FAIL, RETRIES, ERROR:...
	Attempts int    // Try number
}

type Job struct {
	Name        string
	TaskName    string
	NumJobs     int
	Concurrency int
	NumRetries  int

	ID           string
	Tasks        []*Status
	NumRunning   int
	NumCompleted int
	NumPassed    int
}

var Jobs = map[string]*Job{}

func Controller() {
	for {
		now := time.Now()
		for jName, job := range Jobs {
			if job.NumCompleted == job.NumJobs {
				// All done so delete it
				// delete(Jobs, jName)
			}

			// Look for timed-out jobs
			for tID, status := range job.Tasks {
				// Skip jobs that are not running
				if status.Ping.IsZero() {
					continue
				}

				if now.Sub(status.Ping) > restartTimeout {
					log.Printf("Thinks it's still running\n")
					if status.Attempts <= job.NumRetries {
						// Retry
						job.NumRunning--
						StartTask(jName, tID)
					} else {
						// Retried-out
						status.Ping = time.Time{}
						status.End = time.Now()
						status.Status = "RETRIES"
						job.NumCompleted++
						job.NumRunning--
					}
				}
			}

			// Now if we have room, find one to run
			if job.NumCompleted != job.NumJobs && job.NumRunning < job.Concurrency {
				for tID, status := range job.Tasks {
					// Skip jobs that are done or running
					if !status.Ping.IsZero() || status.Status != "" {
						continue
					}

					StartTask(jName, tID)

					// Exit if we're at max concurrency
					if job.NumRunning == job.Concurrency {
						break
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
		// time.Sleep(5 * time.Second)
	}
}

func StartTask(jName string, tID int) {
	Jobs[jName].Tasks[tID].Ping = time.Now()
	Jobs[jName].Tasks[tID].Start = time.Now()
	Jobs[jName].Tasks[tID].Status = ""
	Jobs[jName].Tasks[tID].Attempts++
	Jobs[jName].NumRunning++

	go func() {
		log.Printf("Start task: %d\n", tID)
		// _, err := curl(fmt.Sprintf("http://%s-default.kndev.us-south.containers.appdomain.cloud?KN_JOB_NAME=%s&KN_JOB_INDEX=%d", Jobs[jName].TaskName, jName, tID))
		_, err := curl(fmt.Sprintf("http://%s.default.svc.cluster.local?KN_JOB_NAME=%s&KN_JOB_INDEX=%d", Jobs[jName].TaskName, jName, tID))

		if err != nil {
			Jobs[jName].Tasks[tID].Ping = time.Time{}
			Jobs[jName].Tasks[tID].End = time.Now()
			Jobs[jName].Tasks[tID].Status = "ERROR: " + err.Error()
			Jobs[jName].NumRunning--
		}
	}()
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
		taskName := r.URL.Query().Get("task")
		numJobs := r.URL.Query().Get("num")
		concurrency := r.URL.Query().Get("concurrency")
		retry := r.URL.Query().Get("retry")

		if taskName == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing 'task'\n"))
			return
		}

		var err error
		id := fmt.Sprintf("%d", time.Now().UnixNano()) // fix

		if jobName == "" {
			jobName = id
		}

		job := Job{
			Name:        jobName,
			TaskName:    taskName,
			NumJobs:     1,
			Concurrency: 10,
			NumRetries:  0,

			ID: id,
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

		if concurrency != "" {
			if job.Concurrency, err = strconv.Atoi(concurrency); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad 'concurrency'\n" + err.Error()))
				return
			}
			if job.Concurrency < 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid 'concurrency' value: " + concurrency + "\n"))
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

		job.Tasks = make([]*Status, job.NumJobs)
		for i := range job.Tasks {
			job.Tasks[i] = &Status{}
		}

		Jobs[job.Name] = &job

		r.Header.Add("JOB-NAME", job.Name)
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		buf := []byte{}
		if jobName == "" {
			buf, _ = json.MarshalIndent(Jobs, "", "  ")
		} else {
			job, ok := Jobs[jobName]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Missing job '" + jobName + "'"))
				return
			} else {
				buf, _ = json.MarshalIndent(job, "", "  ")
			}
			if job.NumCompleted == job.NumJobs {
				delete(Jobs, jobName)
			}
		}
		w.Write(buf)
	})

	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		jobName := r.URL.Query().Get("job")
		taskIndex := r.URL.Query().Get("index")
		jobStatus := r.URL.Query().Get("status")
		job, ok := Jobs[jobName]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing job '" + jobName + "'"))
			return
		}

		index, err := strconv.Atoi(taskIndex)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Can't parsing:" + taskIndex + ":" + err.Error()))
			return
		}

		if index > len(job.Tasks) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Index " + taskIndex + " is too big"))
			return
		}

		if job.Tasks[index].Status != "" {
			return
		}

		if jobStatus == "pass" {
			log.Printf("Got PASS %s %d\n", jobName, index)
			job.Tasks[index].Ping = time.Time{}
			job.Tasks[index].End = time.Now()
			job.Tasks[index].Status = "PASS"
			job.NumCompleted++
			job.NumPassed++
			job.NumRunning--
		} else if jobStatus == "fail" {
			if job.Tasks[index].Attempts <= job.NumRetries {
				// Retry
				job.NumRunning--
				StartTask(job.Name, index)
				return
			}
			log.Printf("Got FAIL %s %d\n", jobName, index)
			job.Tasks[index].Ping = time.Time{}
			job.Tasks[index].End = time.Now()
			job.Tasks[index].Status = "FAIL"
			job.NumCompleted++
			job.NumRunning--
		} else {
			log.Printf("Got PING %s %d\n", jobName, index)
			job.Tasks[index].Ping = time.Now()
		}
	})

	log.Print("Job controller listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}
