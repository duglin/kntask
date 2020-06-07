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

	"github.com/gorilla/websocket"
)

func main() {
	log.SetFlags(log.Ltime)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// log.Printf("Got a request\n")

		taskCmd := []string{}
		taskEnv := os.Environ()

		doStream := false
		for _, v := range r.Header["Upgrade"] {
			if strings.HasPrefix(v, "websocket") {
				doStream = true
				taskEnv = append(taskEnv, "K_STREAM=true")
				break
			}
		}

		// os.Args[1] is a JSON serialization of the ENTRYPOINT cmd
		if len(os.Args) > 1 {
			taskCmd = os.Args[1:]
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

			// Grab all K_ ones - only assume only one value
			if strings.HasPrefix(name, "K_") {
				taskEnv = append(taskEnv, name+"="+values[0])
			}

			// Grab all CloudEvent ones - only assume only one value
			if strings.HasPrefix(name, "CE-") {
				taskEnv = append(taskEnv,
					strings.ReplaceAll(name, "-", "_")+"="+values[0])
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

		if len(taskCmd) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Missing command to run\n"))
			return
		}

		tmpURL := r.URL
		tmpURL.Host = r.Host

		headersJson, _ := json.Marshal(r.Header)
		taskEnv = append(taskEnv, "K_HEADERS="+string(headersJson))
		taskEnv = append(taskEnv, "K_URL="+tmpURL.String())
		taskEnv = append(taskEnv, "K_METHOD="+r.Method)

		var outBuf bytes.Buffer
		var outWr io.Writer
		var inRd io.Reader
		var conn *websocket.Conn
		var err error

		if !doStream {
			body := []byte{}
			if r.Body != nil {
				body, _ = ioutil.ReadAll(r.Body)
			}

			inRd = bytes.NewReader(body)
			outBuf = bytes.Buffer{}
			outWr = bufio.NewWriter(&outBuf)
		} else {
			upgrader := websocket.Upgrader{}
			conn, err = upgrader.Upgrade(w, r, nil)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error() + "\n"))
				return
			}
			defer conn.Close()
		}

		cmd := exec.Cmd{
			Path: taskCmd[0],
			Args: taskCmd[0:],
			Env:  taskEnv,
			// Stdin:  inRd,  // bytes.NewReader(body),
			// Stdout: outWr, // os.Stdout, // buffer these
			// Stderr: outWr, // os.Stderr,
		}

		if !doStream {
			cmd.Stdin = inRd
			cmd.Stdout = outWr
			cmd.Stderr = outWr
			err = cmd.Run()
		} else {
			stdin, _ := cmd.StdinPipe()
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			go func() {
				for {
					_, p, err := conn.ReadMessage()
					if len(p) > 0 {
						stdin.Write(p)
						stdin.Write([]byte("\n"))
					}
					if err != nil {
						break
					}
				}
				stdin.Close()
			}()
			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := stdout.Read(buf)
					if n <= 0 && err != nil {
						break
					}
					outBuf.Write([]byte(buf[:n]))
					s := []byte(strings.TrimRight(string(buf[:n]), "\n\r"))
					conn.WriteMessage(websocket.TextMessage, s)
				}
				stdin.Close()
			}()
			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := stderr.Read(buf)
					if n <= 0 && err != nil {
						break
					}
					outBuf.Write([]byte(buf[:n]))
					s := []byte(strings.TrimRight(string(buf[:n]), "\n\r"))
					conn.WriteMessage(websocket.TextMessage, s)
				}
				stdin.Close()
			}()

			err = cmd.Start()
			if err == nil {
				err = cmd.Wait()
			}
			log.Printf("Stream ended\n")
		}
		// 'err' is any possible error from trying to run the command

		// err := cmd.Run()
		if err == nil { // Worked
			if !doStream {
				w.WriteHeader(http.StatusOK)
			}
		} else { // Command failed
			// jobName := r.Header.Get("K_JOB_NAME")
			// log.Printf("Error(%s/%s,%s): %s\n", jobName, jobID, index, err)
			if !doStream {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error() + "\n"))
			}
		}

		if outBuf.Len() > 0 {
			if !doStream {
				w.Write(outBuf.Bytes())
			}
			log.Printf("Output:\n%s\n", string(outBuf.Bytes()))
		}
	})

	// log.Print("Taskmgr listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}
