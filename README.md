Put kn-job into $HOME/.kn/plugins/

Then use: `kn job ...` to do a batch job.

To create a "task" add the label to your KnService - e.g.:
```
kn service create test --image duglin/app -l type=task
```

When running one of the demos, just press any key to run the next command.

KUBECONFIG env var must be set appropriately.
`kubectl` must be available

edit cm/config-autoscaler:
  target-burst-capacity: "0"
  container-concurrency-target-percentage: "100"

https://cloud.ibm.com/docs/containers?topic=containers-file_storage
https://cloud.ibm.com/docs/containers?topic=containers-storage_planning#choose_storage_solution
https://docs.microsoft.com/en-us/azure/batch/tutorial-rendering-cli
https://ibm.ent.box.com/notes/573702339552

