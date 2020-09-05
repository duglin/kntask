source .demoscript

kn service delete stream test stream pulltest hack-job >/dev/null 2>&1
kn job delete --all

comment We start with a simple app
doit cat hack-app

comment --pause And the Dockerfile used to build its image
doit --showcmd="cat Dockerfile" Dockerize -n hack-app

comment --pause "Deploy it... (as a 'task')"
doit kn service create test --image duglin/hack-app -l type=task
URL=$(tail -1 out)

comment --pause "Now hit it - it sleeps for 1 sec"
doit curl -s ${URL}

comment --pause "Now hit it again with an arg"
doit curl -s ${URL}/5

comment --pause "One more time, with feelings"
doit curl -s "${URL}/0?c\\&flag=value"

# comment --pause Check pods
# doit kubectl get pods

comment --pause "Load it up (10 clients)"
doit ./load -l 10 0 ${URL}

# comment --pause "See pods didn't scale"
# doit kubectl get pods

comment --pause Now set concurrency to 1
doit kn service update test --concurrency-limit=1

comment --pause "Load it up again - sleeping 3 each time"
doit ./load -l 10 0 ${URL}/3

# comment --pause "See pods DID scale"
# doit kubectl get pods

comment "Demo streaming app"
doit cat hack-stream

doit kn service create stream --image duglin/hack-stream -l type=task
URL=$(tail -1 out)

comment --pause "Demo streaming"
doit --ignorerc timeout 10s ./client ${URL}

comment "Demo pull/keda style"
doit cat hack-pull
doit kn service create pulltest --image duglin/hack-pull -l type=pull
URL=$(tail -1 out)
comment --pause "Queue has 5 messages added, one per second"
comment "Run and watch the logs"
doit "curl -HPrefer:respond-async -s ${URL}/queueA" # ?async"
skip=1 doit --noscroll --ignorerc "timeout 8 kubectl logs -f -l serving.knative.dev/service=pulltest -c user-container"

comment --pause "Jobs jobs jobs"

doit cat hack-job
comment "Notice it will fail 1/3 of the time"

comment "Deploy the service (task) - as normal"
doit kn service create hack-job --image duglin/hack-job -l type=task

comment "Now create our batch job (run service 5 times, no retries)"
doit kn job create hack-job --service=hack-job --num=5
doit --noexec klog hack-job
kubectl logs -l serving.knative.dev/service=hack-job -c user-container

comment "Now wait for it to finish"
doit kn job wait hack-job
doit kn job get hack-job
comment "Notice that some should have failed"

doit kn job create hack-job2 --service=hack-job/arg1 --num=50 --retry=-1 --parallel=10 -w -- arg2 arg3
doit kn job get hack-job2 \| head -20
doit kn job get hack-job2 \| grep Attempts
doit --noexec klog hack-job
kubectl logs --tail=99 -l serving.knative.dev/service=hack-job -c user-container

comment --pause "Clean..."
doit kn service delete test test2 pulltest stream
doit kn job del --all
