package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

/*
# kn job create MYJOB --task=MYTASK --num=# --parallel=# --retry=# \
#                     --flavor=FLAVOR --env --wait/-w
# kn job wait MYJOB
# kn job status MYJOB
*/

var jobName string
var taskName string
var num int
var parallel int
var retry int
var flavor string
var envs []string
var wait bool
var args []string

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

func createFunc(cmd *cobra.Command, args []string) {
	dashdash := cmd.Flags().ArgsLenAtDash()
	jobName = args[0]
	taskArgs := []string{}

	if dashdash == 0 {
		fmt.Printf("Missing JOB\n")
		os.Exit(1)
	} else if dashdash > 0 {
		taskArgs = args[dashdash:]
	}

	if num <= 0 {
		fmt.Printf("Invalid '--num' value: %d\n", num)
		os.Exit(1)
	}
	if parallel < 1 {
		fmt.Printf("Invalid '--parallel' value: %d\n", parallel)
		os.Exit(1)
	}
	if retry < 0 {
		fmt.Printf("Invalid '--retry' value: %d\n", retry)
		os.Exit(1)
	}

	u := "http://" + host + "/create?"
	u += "job=" + jobName
	u += "&task=" + taskName
	if num > 1 {
		u += fmt.Sprintf("&num=%d", num)
	}
	if parallel > 1 {
		u += fmt.Sprintf("&parallel=%d", parallel)
	}
	if retry > 0 {
		u += fmt.Sprintf("&retry=%d", retry)
	}
	if flavor != "" {
		u += fmt.Sprintf("&flavor=%s", flavor)
	}
	for _, e := range envs {
		u += fmt.Sprintf("&env=%s", url.QueryEscape(e))
	}

	headers := [][2]string{}
	for i, arg := range taskArgs {
		headers = append(headers, [2]string{fmt.Sprintf("ARG_%d", i+1), arg})
	}

	res, err := curl(u, headers)
	if len(res) > 0 {
		fmt.Printf("%s\n", res)
	}
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	if wait {
		waitFunc(cmd, []string{jobName})
	}
}

func waitFunc(cmd *cobra.Command, args []string) {
	u := "http://" + host + "/status?job=" + args[0]

	for {
		res, err := curl(u, nil)
		if strings.Contains(res, "find job") {
			fmt.Printf("%s\n", res)
			os.Exit(1)
		}

		status := struct {
			NumCompleted int
			NumJobs      int
		}{}
		err = json.Unmarshal([]byte(res), &status)
		if err != nil {
			fmt.Printf("Error parsing: %s\n", err)
			os.Exit(1)
		}
		if status.NumCompleted == status.NumJobs {
			fmt.Printf("%s\n", res)
			break
		}
	}

}

func statusFunc(cmd *cobra.Command, args []string) {
	u := "http://" + host + "/status?"

	if len(args) > 0 {
		u += "job=" + args[0]
	}

	res, err := curl(u, nil)
	fmt.Printf("%s\n", res)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		fmt.Printf("%s\n", res)
		os.Exit(1)
	}

}

func main() {
	createCmd := &cobra.Command{
		Use:   "create MYJOB",
		Short: "Create a new Job",
		Long:  "",
		Args:  cobra.MinimumNArgs(1),
		Run:   createFunc,
	}

	createCmd.Flags().StringVarP(&taskName, "task", "t", "",
		"Name of task to run")
	createCmd.MarkFlagRequired("task")
	createCmd.Flags().IntVarP(&num, "num", "n", 1,
		"Number of tasks in the Job")
	createCmd.Flags().IntVarP(&parallel, "parallel", "p", 1,
		"Max number of tasks to run at one time")
	createCmd.Flags().IntVarP(&retry, "retry", "r", 0,
		"Number of times to retry a failed task")
	createCmd.Flags().StringVarP(&flavor, "flavor", "f", "",
		"Flavor of VM to allocate for tasks")
	createCmd.Flags().StringArrayVarP(&envs, "env", "e", nil,
		"Add env var(s) to task")
	createCmd.Flags().BoolVarP(&wait, "wait", "w", false,
		"Wait for Job to complete")

	waitCmd := &cobra.Command{
		Use:   "wait MYJOB",
		Short: "Wait for a Job to complete",
		Long:  "",
		Args:  cobra.ExactArgs(1),
		Run:   waitFunc,
	}

	statusCmd := &cobra.Command{
		Use:   "status [ MYJOB ]",
		Short: "Get the status of a Job, or all Jobs",
		Long:  "",
		Args:  cobra.MaximumNArgs(1),
		Run:   statusFunc,
	}

	cmd := &cobra.Command{Use: fmt.Sprintf("kn%cjob", 011)}
	cmd.AddCommand(createCmd, waitCmd, statusCmd)
	cmd.Execute()
}
