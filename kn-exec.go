package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

/*
# kn exec SERVICE --name NAME --num=# --parallel=# --retry=# \
#                     --env --async/-a --wait=NAME
# kn exec --wait NAME
# kn exec --status NAME
*/

var jobName string
var serviceName string
var dependsOn string
var num int
var parallel int
var retry int
var envs []string
var async bool

var wait string
var status string
var clear bool

var host = "jobcontroller-default.kndev.us-south.containers.appdomain.cloud"

func curl(url string, headers [][2]string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)

	for _, header := range headers {
		req.Header.Add(header[0], header[1])
	}

	res, err := client.Do(req)
	body := ""
	if res != nil && res.Body != nil {
		var buf = []byte{}
		buf, _ = ioutil.ReadAll(res.Body)
		body = string(buf)
		res.Body.Close()
	}
	return body, err
}

func execFunc(cmd *cobra.Command, args []string) {
	dashdash := cmd.Flags().ArgsLenAtDash()
	serviceArgs := []string{}
	flags := cmd.Flags()

	if dashdash >= 0 {
		serviceArgs = args[dashdash:]
		args = args[:dashdash]
	}

	if len(args) > 1 {
		fmt.Printf("Only one SERVICE can be specified at a time\n")
		os.Exit(1)
	}

	if len(args) == 1 {
		serviceName = args[0]

		if wait != "" {
			fmt.Printf("Can't specify SERVICE and --wait together\n")
			os.Exit(1)
		}

		if status != "" {
			fmt.Printf("Can't specify SERVICE and --status together\n")
			os.Exit(1)
		}
	} else if wait != "" && status != "" {
		fmt.Printf("Can't specify --wait and --status together\n")
		os.Exit(1)
	} else if wait != "" {
		doWait(wait)
		return
	} else if status != "" {
		doStatus(status)
		return
	} else {
		fmt.Printf("Missing SERVICE or --wait or --status\n")
		os.Exit(1)
	}

	// OK we're executing a job!

	// If any of these were specified then we're doing a batch
	if !flags.Changed("name") && !flags.Changed("num") &&
		!flags.Changed("retry") &&
		!flags.Changed("parallel") && !flags.Changed("async") {

		query := ""
		parts := strings.SplitN(serviceName, "?", 2)
		if len(parts) == 2 {
			serviceName = parts[0]
			query = "?" + parts[1]
		}

		parts = strings.SplitN(serviceName, "/", 2)
		if len(parts) == 2 {
			serviceName = parts[0]
			query = "/" + parts[1] + query
		}

		// Not a batch job, just a normal curl
		Cmd := exec.Command("kubectl", "get", "ksvc/"+serviceName,
			"-o", "go-template={{.status.url}}")
		output, err := Cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("%s\n", output)
			os.Exit(1)
		}
		host := string(output) + query

		for {
			result, err := curl(host, nil)
			if err == nil {
				fmt.Printf("%s", result)
				return
			}
			if retry > 0 {
				retry--
				continue
			}
			fmt.Printf("%s\n", err)
			os.Exit(1)
		}
	}

	if num <= 0 {
		fmt.Printf("Invalid '--num' value: %d\n", num)
		os.Exit(1)
	}
	if parallel < 1 {
		fmt.Printf("Invalid '--parallel' value: %d\n", parallel)
		os.Exit(1)
	}
	if retry < -1 {
		fmt.Printf("Invalid '--retry' value: %d\n", retry)
		os.Exit(1)
	}

	if jobName == "" {
		jobName = fmt.Sprintf("%d", time.Now().Unix())
	}

	u := "http://" + host + "/create?"
	u += "job=" + jobName
	u += "&service=" + serviceName
	if num > 1 {
		u += fmt.Sprintf("&num=%d", num)
	}
	if parallel > 1 {
		u += fmt.Sprintf("&parallel=%d", parallel)
	}
	// if retry > 0 {
	u += fmt.Sprintf("&retry=%d", retry)
	// }
	for _, e := range envs {
		u += fmt.Sprintf("&env=%s", url.QueryEscape(e))
	}
	if dependsOn != "" {
		u += fmt.Sprintf("&dependson=%s", url.QueryEscape(dependsOn))
	}

	headers := [][2]string{}
	for i, arg := range serviceArgs {
		headers = append(headers, [2]string{fmt.Sprintf("ARG_%d", i+1), arg})
	}

	// fmt.Printf("URL: %s\n", u)

	res, err := curl(u, headers)
	if len(res) > 0 {
		fmt.Printf("%s", res)
	}
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	if !async {
		doWait(jobName)
	} else {
		fmt.Printf("%s started\n", jobName)
	}
}

func doWait(name string) {
	u := "http://" + host + "/status?job=" + name + "&clear"

	for {
		res, err := curl(u, nil)
		if strings.Contains(res, "find job") {
			fmt.Printf("%s", res)
			os.Exit(1)
		}

		status := struct {
			NumCompleted int
			NumJobs      int
		}{}
		err = json.Unmarshal([]byte(res), &status)
		if err != nil {
			fmt.Printf("Error parsing(%s):\n%s\n%s\n", u, res, err)
			os.Exit(1)
		}
		if status.NumCompleted == status.NumJobs {
			fmt.Printf("%s", res)
			break
		}
	}

}

func doStatus(name string) {
	if name == "all" {
		name = ""
	}

	u := "http://" + host + "/status?"

	if name != "" {
		u += "job=" + name
	}

	if clear {
		u += "&clear"
	}

	res, err := curl(u, nil)
	if len(res) > 0 {
		fmt.Printf("%s", res)
	}
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

}

func main() {
	Cmd := exec.Command("kubectl", "get", "cm/ibm-cloud-cluster-ingress-info",
		"-n", "kube-system",
		"-o", `go-template={{index .data "ingress-subdomain" }}`)
	output, err := Cmd.Output()
	if err != nil {
		fmt.Printf("Can't determine the URL of the cluster: %s\n", err)
		if len(output) > 0 {
			fmt.Printf("%s\n", string(output))
		}
		os.Exit(1)
	}
	host = "jobcontroller-default." + string(output)

	execCmd := &cobra.Command{
		Use:                   "exec [SERVICE] [flags] [ -- SERVICE_ARGS... ]",
		Short:                 "Execute a service",
		Args:                  cobra.MinimumNArgs(0),
		Run:                   execFunc,
		DisableFlagsInUseLine: true,
	}

	execCmd.Flags().StringVarP(&jobName, "name", "", "",
		"Name of execution")
	execCmd.Flags().IntVarP(&num, "num", "n", 1,
		"Number of times to run the Service")
	execCmd.Flags().IntVarP(&parallel, "parallel", "p", 1,
		"Max number of services calls to run at one time")
	execCmd.Flags().IntVarP(&retry, "retry", "r", 0,
		"Number of times to retry a failed service call")
	execCmd.Flags().StringArrayVarP(&envs, "env", "e", nil,
		"Add env var(s) to service")
	execCmd.Flags().BoolVarP(&async, "async", "a", false,
		"Execution it asynchronous, do not wait for it to complete")
	execCmd.Flags().StringVarP(&dependsOn, "dependson", "d", "",
		"Dependent NAME[:fail|pass]")

	execCmd.Flags().StringVarP(&wait, "wait", "w", "",
		"Wait for the specified execution to complete")
	execCmd.Flags().StringVarP(&status, "status", "s", "",
		"Return the status of the specified execution")
	execCmd.Flags().BoolVarP(&clear, "clear", "c", false,
		"Erase status of queried executions\n")

	// cmd := &cobra.Command{Use: fmt.Sprintf("kn%cexec", 011)}
	// cmd.AddCommand(execCmd)
	// cmd.Execute()
	execCmd.Execute()
}
