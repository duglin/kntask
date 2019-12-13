package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

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
	taskCmd := []string{"/app"}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// log.Printf("Got a request\n")
		body := []byte{}
		if r.Body != nil {
			body, _ = ioutil.ReadAll(r.Body)
		}

		headersJson, _ := json.Marshal(r.Header)
		index := r.URL.Query().Get("KN_JOB_INDEX")
		jobName := r.URL.Query().Get("KN_JOB_NAME")

		taskEnv := os.Environ()
		taskEnv = append(taskEnv, "KN_TASK_HEADERS="+string(headersJson))
		taskEnv = append(taskEnv, "KN_TASK_URL="+r.URL.String())
		if jobName != "" {
			taskEnv = append(taskEnv, "KN_JOB_NAME="+jobName)
		}
		if index != "" {
			taskEnv = append(taskEnv, "KN_JOB_INDEX="+index)
		}

		done := false
		if jobName != "" && index != "" {
			go func() {
				for !done {
					updateJob(jobName, index, "") // Ping
					time.Sleep(5 * time.Second)
				}
			}()
		}

		// log.Printf("Stdin buf: %s\n", string(body))
		outBuf := bytes.Buffer{}
		outWr := bufio.NewWriter(&outBuf)
		cmd := exec.Cmd{
			Path:   taskCmd[0],
			Args:   taskCmd[1:],
			Env:    taskEnv,
			Stdin:  bytes.NewReader(body),
			Stdout: outWr, // os.Stdout, // buffer these
			Stderr: outWr, // os.Stderr,
		}

		// outWr.Flush()

		err := cmd.Run()
		done = true
		if err == nil {
			// Worked
			log.Printf("Ran ok (%s,%s)\n", jobName, index)
			updateJob(jobName, index, "pass")
			w.WriteHeader(http.StatusOK)
			w.Write(outBuf.Bytes())
		} else {
			log.Printf("Error(%s,%s): %s\n", jobName, index, err)
			updateJob(jobName, index, "fail")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error() + "\n"))
			w.Write(outBuf.Bytes())
		}
		log.Printf("Output:\n%s\n", string(outBuf.Bytes()))
	})

	log.Print("Taskmgr listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}

func updateJob(job string, index string, status string) {
	if job == "" || index == "" {
		return
	}
	// domain := "kndev.us-south.containers.appdomain.cloud"
	// url := "http://jobcontroller-default." + domain
	url := "http://jobcontroller.default.svc.cluster.local"
	if status != "" {
		status = "&status=" + status
	}
	cmd := fmt.Sprintf("%s/update?job=%s&index=%s%s", url, job, index, status)
	_, err := curl(cmd)
	if err != nil {
		log.Printf("Curl: %s\n", err)
	}
}
