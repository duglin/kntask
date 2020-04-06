all: .d-taskmgr .d-app .d-app-hi .d-app-echo .d-jobcontroller load kn-job \
	client kn-exec

taskmgr: taskmgr.go
	GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 go build \
   	-ldflags "-s -w -extldflags -static" \
	-tags netgo -installsuffix netgo \
	-o taskmgr taskmgr.go

client: client.go
	go build -o client client.go
	# GOOS=darwin GOARCH=amd64 go build -o client.mac load.go

.d-taskmgr: taskmgr.go pullmgr.go
	./Dockerize taskmgr.go pullmgr.go

.d-jobcontroller: jobcontroller.go
	./Dockerize jobcontroller.go

.d-app: app
	./Dockerize app

.d-app-hi: app-hi
	./Dockerize app-hi

.d-app-echo: app-echo
	./Dockerize app-echo

run: .app
	docker run -ti -p 8080:8080 duglin/app

deploy: .d-jobcontroller .d-taskmgr .d-app
	kubectl delete ksvc --all > /dev/null 2>&1 || true
	sleep 2
	kn service create jobcontroller --image duglin/jobcontroller --min-scale=1
	./prep
	# kubectl create -f s.yaml > /dev/null 2>&1
	kn service create test --image duglin/app --min-scale=1 \
		--concurrency-limit=1 -l type=task
	kn service delete test

load: load.go
	go build -o load load.go
	# GOOS=darwin GOARCH=amd64 go build -o load.mac load.go

kn-job: kn-job.go
	go build -o kn-job kn-job.go
	# GOOS=darwin GOARCH=amd64 go build -o kn-job.mac kn-job.go

kn-exec: kn-exec.go
	go build -o kn-exec kn-exec.go
	# GOOS=darwin GOARCH=amd64 go build -o kn-exec.mac kn-exec.go

clean:
	rm -f jobcontroller load taskmgr kn-job client *.mac kn-exec .d-*
	kubectl delete ksvc --all > /dev/null 2>&1 || true
