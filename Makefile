all: .taskmgr .app .jobcontroller load kn-job client

taskmgr: taskmgr.go
	GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 go build \
   	-ldflags "-s -w -extldflags -static" \
	-tags netgo -installsuffix netgo \
	-o taskmgr taskmgr.go

client: client.go
	go build -o client client.go
	GOOS=darwin GOARCH=amd64 go build -o client.mac load.go

.taskmgr: taskmgr.go Dockerfile.taskmgr pullmgr.go
	go build -o /dev/null taskmgr.go # quick fail
	go build -o /dev/null pullmgr.go # quick fail
	docker build -f Dockerfile.taskmgr -t duglin/taskmgr .
	docker push duglin/taskmgr
	touch .taskmgr

.jobcontroller: jobcontroller.go Dockerfile.jobcontroller
	go build -o /dev/null jobcontroller.go # quick fail
	docker build -f Dockerfile.jobcontroller -t duglin/jobcontroller .
	docker push duglin/jobcontroller
	touch .jobcontroller

.app: app Dockerfile.app 
	docker build -f Dockerfile.app -t duglin/app .
	docker push duglin/app
	touch .app

run: .app
	docker run -ti -p 8080:8080 duglin/app

deploy: .jobcontroller .taskmgr .app
	kubectl delete ksvc --all > /dev/null 2>&1 || true
	sleep 2
	kn service create jobcontroller --image duglin/jobcontroller --min-scale=1
	./prep
	# kubectl create -f s.yaml > /dev/null 2>&1
	kn service create test --image duglin/app --min-scale=1 \
		--concurrency-limit=1 -l type=task

load: load.go
	go build -o load load.go
	GOOS=darwin GOARCH=amd64 go build -o load.mac load.go

kn-job: kn-job.go
	go build -o kn-job kn-job.go
	GOOS=darwin GOARCH=amd64 go build -o kn-job.mac kn-job.go

clean:
	rm -f jobcontroller load taskmgr kn-job client *.mac
	kubectl delete ksvc --all > /dev/null 2>&1 || true
