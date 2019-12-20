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
	"strings"
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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// log.Printf("Got a request\n")

		taskCmd := []string{"/app"}

		body := []byte{}
		if r.Body != nil {
			body, _ = ioutil.ReadAll(r.Body)
		}

		index := r.Header.Get("K_JOB_INDEX")
		jobName := r.Header.Get("K_JOB_NAME")
		jobID := r.Header.Get("K_JOB_ID")

		taskEnv := os.Environ()
		for name, values := range r.Header {
			name = strings.ToUpper(name)

			// Not sure this can ever happen but just in case
			if len(values) == 0 {
				continue
			}

			// Env vars are copied and here could be lots
			if name == "K_ENV" {
				for _, value := range values {
					log.Printf("Adding env: %s\n", value)
					taskEnv = append(taskEnv, value)
				}
				continue
			}

			if strings.HasPrefix(name, "K_ARG_") {
				continue
			}

			// All others are just copied - only assume only one value
			if strings.HasPrefix(name, "K_") {
				taskEnv = append(taskEnv, name+"="+values[0])
			}
		}

		for i := 1; ; i++ {
			arg, ok := r.Header[fmt.Sprintf("K_arg_%d", i)]
			if !ok {
				break
			}
			taskCmd = append(taskCmd, arg[0])
		}

		headersJson, _ := json.Marshal(r.Header)
		taskEnv = append(taskEnv, "K_HEADERS="+string(headersJson))

		// // taskEnv = append(taskEnv, "K_TASK_URL="+r.URL.String())
		// if jobName != "" {
		// taskEnv = append(taskEnv, "K_JOB_NAME="+jobName)
		// }
		// if jobID != "" {
		// taskEnv = append(taskEnv, "K_JOB_ID="+jobID)
		// }
		// if index != "" {
		// taskEnv = append(taskEnv, "K_JOB_INDEX="+index)
		// }

		done := false
		if jobID != "" && index != "" {
			go func() {
				for !done {
					updateJob(jobID, index, "") // Ping
					time.Sleep(5 * time.Second)
				}
			}()
		}

		// log.Printf("Stdin buf: %s\n", string(body))
		outBuf := bytes.Buffer{}
		outWr := bufio.NewWriter(&outBuf)
		cmd := exec.Cmd{
			Path:   taskCmd[0],
			Args:   taskCmd[0:],
			Env:    taskEnv,
			Stdin:  bytes.NewReader(body),
			Stdout: outWr, // os.Stdout, // buffer these
			Stderr: outWr, // os.Stderr,
		}

		err := cmd.Run()
		done = true
		if err == nil {
			// Worked
			log.Printf("Ran ok (%s/%s,%s)\n", jobName, jobID, index)
			updateJob(jobID, index, "pass")
			w.WriteHeader(http.StatusOK)
			w.Write(outBuf.Bytes())
		} else {
			log.Printf("Error(%s/%s,%s): %s\n", jobName, jobID, index, err)
			updateJob(jobID, index, "fail")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error() + "\n"))
			w.Write(outBuf.Bytes())
		}
		log.Printf("Output:\n%s\n", string(outBuf.Bytes()))
	})

	// log.Print("Taskmgr listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}

func updateJob(jobID string, index string, status string) {
	if jobID == "" || index == "" {
		return
	}
	// domain := "kndev.us-south.containers.appdomain.cloud"
	// url := "http://jobcontroller-default." + domain
	url := "http://jobcontroller.default.svc.cluster.local"
	if status != "" {
		status = "&status=" + status
	}
	cmd := fmt.Sprintf("%s/update?job=%s&index=%s%s", url, jobID, index, status)
	res, err := curl(cmd)
	if err != nil {
		log.Printf("Curl: %s | %s\n", err, res)
	}
}
