To create a "task" add the label to your KnService - e.g.:
```
kn service create test --image duglin/app -l type=task
```

Put `kn-job` into `$HOME/.kn/plugins/`

Then use: `kn job ...` to do a batch job.
Make sure you start the job controller for batch jobs first:
```
kn service create jobcontroller --image duglin/jobcontroller --scale=1
```
Eventually this should be a real controller with a CRD.

For a demo try: `./hack`.

To send an async request, add the `Prefer:respond-async` header to your
request:
```
curl -HPrefer:respond-async ...
```
You should get a 202 back right away but the request will still be processed
by your service.


For fun try:
```
$ kn service create bash --image ubuntu -l type=task
$ curl http://bash... -d "echo hello world"
hello world
```


When running one of the demos, just press any key to run the next command.

KUBECONFIG env var must be set appropriately.
`kubectl` must be available

Notes for Doug:

edit cm/config-autoscaler:
  target-burst-capacity: "0"
  container-concurrency-target-percentage: "100"

https://cloud.ibm.com/docs/containers?topic=containers-file_storage
https://cloud.ibm.com/docs/containers?topic=containers-storage_planning#choose_storage_solution
https://docs.microsoft.com/en-us/azure/batch/tutorial-rendering-cli
https://ibm.ent.box.com/notes/573702339552

Other "wrappers" we could do:
- event puller (ala keda)
- Request for work/event in general - like Lambda does (apparently)

Move job updated into Queue Proxy

kubectl taint nodes 10.74.199.18 flavor=2x16:NoSchedule

Other options:
- new QP to create pod for each batch job instead of exec
- new QP to create job for each batch job
  - main container could just sleep


TODO:
- add ability to specify which indexes should be run (or re-run) ?
