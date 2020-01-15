package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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
		taskEnv := os.Environ()

		// os.Args[1] is a JSON serialization of the ENTRYPOINT cmd
		if len(os.Args) > 1 {
			if err := json.Unmarshal([]byte(os.Args[1]), &taskCmd); err != nil {
				fmt.Printf("Error parsing cmd: %s\n", err)
				return
			}
		}

		for name, values := range r.Header {
			name = strings.ToUpper(name)

			// Not sure this can ever happen but just in case
			if len(values) == 0 {
				continue
			}

			// Env vars are copied and there could be lots
			if name == "K_ENV" {
				for _, value := range values {
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

		// Use the incoming URL "Path" as the args
		for _, part := range strings.Split(r.URL.Path, "/") {
			part, err := url.PathUnescape(part)
			if part == "" || err != nil {
				continue
			}
			taskCmd = append(taskCmd, part)
		}

		// Use the incoming URL "Query Params" as flags
		for key, values := range r.URL.Query() {
			for _, val := range values {
				// Only single char keys and vals of "" map to - flags
				if val == "" && len(key) == 1 {
					taskCmd = append(taskCmd, fmt.Sprintf("-%s", key))
				} else {
					if val != "" {
						val = "=" + val
					}
					taskCmd = append(taskCmd, fmt.Sprintf("--%s%s", key, val))
				}
			}
		}

		// Append any K_ARG_# env vars as args to the cmd line
		for i := 1; ; i++ {
			arg, ok := r.Header[fmt.Sprintf("K_arg_%d", i)]
			if !ok {
				break
			}
			taskCmd = append(taskCmd, arg[0])
		}

		tmpURL := r.URL
		tmpURL.Host = r.Host

		headersJson, _ := json.Marshal(r.Header)
		taskEnv = append(taskEnv, "K_HEADERS="+string(headersJson))
		taskEnv = append(taskEnv, "K_URL="+tmpURL.String())
		taskEnv = append(taskEnv, "K_METHOD="+r.Method)

		index := r.Header.Get("K_JOB_INDEX")
		jobID := r.Header.Get("K_JOB_ID")
		done := false

		if jobID != "" && index != "" {
			go func() {
				for !done {
					updateJob(jobID, index, "") // Ping
					time.Sleep(5 * time.Second)
				}
			}()
		}

		// Normally we loop until we want to stop reading from Queue
		for i := 0; i < 5; i++ {
			str := fmt.Sprintf("Hello: %s\n", time.Now().Format(time.UnixDate))

			var outBuf bytes.Buffer
			var outWr io.Writer
			var inRd io.Reader
			var err error

			inRd = bytes.NewReader([]byte(str))
			outBuf = bytes.Buffer{}
			outWr = bufio.NewWriter(&outBuf)

			cmd := exec.Cmd{
				Path: taskCmd[0],
				Args: taskCmd[0:],
				Env:  taskEnv,
				// Stdin:  inRd,  // bytes.NewReader(body),
				// Stdout: outWr, // os.Stdout, // buffer these
				// Stderr: outWr, // os.Stderr,
			}

			cmd.Stdin = inRd
			cmd.Stdout = outWr
			cmd.Stderr = outWr
			err = cmd.Run()
			// 'err' is any possible error from trying to run the command

			if err == nil { // Worked
				fmt.Printf("Passed\n")
			} else { // Command failed
				fmt.Printf("Failed\n")
			}

			if outBuf.Len() > 0 {
				log.Printf("Output:\n%s\n", string(outBuf.Bytes()))
			}

			time.Sleep(5 * time.Second)
		}

		done = true

		// 1/2 the time fail
		if time.Now().Unix()%2 == 0 {
			updateJob(jobID, index, "pass")
		} else {
			updateJob(jobID, index, "fail")
		}
	})

	http.ListenAndServe(":8080", nil)
}

func updateJob(jobID string, index string, status string) {
	if jobID == "" || index == "" {
		return
	}
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
